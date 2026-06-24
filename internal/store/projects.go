package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// DefaultStatuses is the board seeded for every new project: one column per
// category, the minimum that makes a kanban board meaningful. Projects can add
// or rename columns later.
func DefaultStatuses() []core.Status {
	return []core.Status{
		{Name: "To Do", Category: core.CategoryTodo, Position: 0},
		{Name: "In Progress", Category: core.CategoryInProgress, Position: 1},
		{Name: "Done", Category: core.CategoryDone, Position: 2},
	}
}

// CreateProject validates and inserts a project, seeds its default board, and
// returns the stored project with its ID and CreatedAt populated. The prefix is
// normalized to upper-case.
func (s *Store) CreateProject(p core.Project) (core.Project, error) {
	p.KeyPrefix = core.NormalizePrefix(p.KeyPrefix)
	if err := p.Validate(); err != nil {
		return core.Project{}, err
	}
	defaultsJSON, err := json.Marshal(p.Defaults)
	if err != nil {
		return core.Project{}, fmt.Errorf("store: marshal defaults: %w", err)
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return core.Project{}, err
	}
	defer tx.Rollback()

	p.CreatedAt = s.now()
	res, err := tx.Exec(
		`INSERT INTO projects (key_prefix, name, repo_path, defaults, seq_counter, created_at)
		 VALUES (?, ?, ?, ?, 0, ?)`,
		p.KeyPrefix, p.Name, p.RepoPath, string(defaultsJSON), fmtTime(p.CreatedAt),
	)
	if err != nil {
		return core.Project{}, fmt.Errorf("store: insert project: %w", err)
	}
	p.ID, err = res.LastInsertId()
	if err != nil {
		return core.Project{}, err
	}

	for _, st := range DefaultStatuses() {
		if _, err := tx.Exec(
			`INSERT INTO statuses (project_id, name, category, position) VALUES (?, ?, ?, ?)`,
			p.ID, st.Name, string(st.Category), st.Position,
		); err != nil {
			return core.Project{}, fmt.Errorf("store: seed status: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return core.Project{}, err
	}
	s.publish(core.Event{Type: core.EventProjectChanged, ProjectID: p.ID})
	return p, nil
}

// scanProject reads a project row in the standard column order.
func scanProject(row interface{ Scan(...any) error }) (core.Project, error) {
	var (
		p            core.Project
		defaultsJSON string
		createdAt    string
	)
	if err := row.Scan(&p.ID, &p.KeyPrefix, &p.Name, &p.RepoPath, &defaultsJSON, &createdAt); err != nil {
		return core.Project{}, err
	}
	if err := json.Unmarshal([]byte(defaultsJSON), &p.Defaults); err != nil {
		return core.Project{}, fmt.Errorf("store: unmarshal defaults: %w", err)
	}
	t, err := parseTime(createdAt)
	if err != nil {
		return core.Project{}, err
	}
	p.CreatedAt = t
	return p, nil
}

const projectCols = `id, key_prefix, name, repo_path, defaults, created_at`

// GetProject returns the project with the given ID.
func (s *Store) GetProject(id int64) (core.Project, error) {
	row := s.db.QueryRow(`SELECT `+projectCols+` FROM projects WHERE id = ?`, id)
	p, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Project{}, ErrNotFound
	}
	return p, err
}

// GetProjectByPrefix returns the project owning the given key prefix.
func (s *Store) GetProjectByPrefix(prefix string) (core.Project, error) {
	row := s.db.QueryRow(`SELECT `+projectCols+` FROM projects WHERE key_prefix = ?`, core.NormalizePrefix(prefix))
	p, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Project{}, ErrNotFound
	}
	return p, err
}

// ListProjects returns every project, oldest first.
func (s *Store) ListProjects() ([]core.Project, error) {
	rows, err := s.db.Query(`SELECT ` + projectCols + ` FROM projects ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpdateProject persists changes to a project's mutable fields (name, repo path,
// defaults). The prefix is immutable because issue keys depend on it.
func (s *Store) UpdateProject(p core.Project) error {
	if err := p.Validate(); err != nil {
		return err
	}
	defaultsJSON, err := json.Marshal(p.Defaults)
	if err != nil {
		return fmt.Errorf("store: marshal defaults: %w", err)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	res, err := s.db.Exec(
		`UPDATE projects SET name = ?, repo_path = ?, defaults = ? WHERE id = ?`,
		p.Name, p.RepoPath, string(defaultsJSON), p.ID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.publish(core.Event{Type: core.EventProjectChanged, ProjectID: p.ID})
	return nil
}

// DeleteProject removes a project and, by cascade, all of its issues, sprints,
// statuses, tags, comments, runs and schedules.
func (s *Store) DeleteProject(id int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	res, err := s.db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.publish(core.Event{Type: core.EventProjectChanged, ProjectID: id})
	return nil
}

// --- statuses ----------------------------------------------------------------

// ListStatuses returns a project's board columns ordered by position.
func (s *Store) ListStatuses(projectID int64) ([]core.Status, error) {
	rows, err := s.db.Query(
		`SELECT id, project_id, name, category, position FROM statuses
		 WHERE project_id = ? ORDER BY position, id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Status
	for rows.Next() {
		var st core.Status
		if err := rows.Scan(&st.ID, &st.ProjectID, &st.Name, &st.Category, &st.Position); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// GetStatus returns a single status by ID.
func (s *Store) GetStatus(id int64) (core.Status, error) {
	var st core.Status
	err := s.db.QueryRow(
		`SELECT id, project_id, name, category, position FROM statuses WHERE id = ?`, id,
	).Scan(&st.ID, &st.ProjectID, &st.Name, &st.Category, &st.Position)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Status{}, ErrNotFound
	}
	return st, err
}

// CreateStatus adds a board column to a project.
func (s *Store) CreateStatus(st core.Status) (core.Status, error) {
	if err := st.Validate(); err != nil {
		return core.Status{}, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	res, err := s.db.Exec(
		`INSERT INTO statuses (project_id, name, category, position) VALUES (?, ?, ?, ?)`,
		st.ProjectID, st.Name, string(st.Category), st.Position,
	)
	if err != nil {
		return core.Status{}, err
	}
	st.ID, err = res.LastInsertId()
	if err != nil {
		return core.Status{}, err
	}
	s.publish(core.Event{Type: core.EventProjectChanged, ProjectID: st.ProjectID})
	return st, nil
}
