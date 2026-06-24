package store

import (
	"database/sql"
	"errors"

	"github.com/03-CiprianoG/jeera/internal/core"
)

const sprintCols = `id, project_id, name, goal, state, start_at, end_at`

func scanSprint(row interface{ Scan(...any) error }) (core.Sprint, error) {
	var (
		sp      core.Sprint
		startAt sql.NullString
		endAt   sql.NullString
	)
	if err := row.Scan(&sp.ID, &sp.ProjectID, &sp.Name, &sp.Goal, &sp.State, &startAt, &endAt); err != nil {
		return core.Sprint{}, err
	}
	var err error
	if sp.StartAt, err = nullToPtrTime(startAt); err != nil {
		return core.Sprint{}, err
	}
	if sp.EndAt, err = nullToPtrTime(endAt); err != nil {
		return core.Sprint{}, err
	}
	return sp, nil
}

// CreateSprint inserts a sprint, defaulting its state to "future".
func (s *Store) CreateSprint(sp core.Sprint) (core.Sprint, error) {
	if sp.State == "" {
		sp.State = core.SprintFuture
	}
	if err := sp.Validate(); err != nil {
		return core.Sprint{}, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	res, err := s.db.Exec(
		`INSERT INTO sprints (project_id, name, goal, state, start_at, end_at) VALUES (?, ?, ?, ?, ?, ?)`,
		sp.ProjectID, sp.Name, sp.Goal, string(sp.State), ptrTimeToNull(sp.StartAt), ptrTimeToNull(sp.EndAt),
	)
	if err != nil {
		return core.Sprint{}, err
	}
	if sp.ID, err = res.LastInsertId(); err != nil {
		return core.Sprint{}, err
	}
	s.publish(core.Event{Type: core.EventSprintChanged, ProjectID: sp.ProjectID})
	return sp, nil
}

// GetSprint returns a sprint by ID.
func (s *Store) GetSprint(id int64) (core.Sprint, error) {
	row := s.db.QueryRow(`SELECT `+sprintCols+` FROM sprints WHERE id = ?`, id)
	sp, err := scanSprint(row)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Sprint{}, ErrNotFound
	}
	return sp, err
}

// ListSprints returns a project's sprints, newest-created first.
func (s *Store) ListSprints(projectID int64) ([]core.Sprint, error) {
	rows, err := s.db.Query(`SELECT `+sprintCols+` FROM sprints WHERE project_id = ? ORDER BY id DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Sprint
	for rows.Next() {
		sp, err := scanSprint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

// UpdateSprint persists changes to a sprint.
func (s *Store) UpdateSprint(sp core.Sprint) error {
	if err := sp.Validate(); err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	res, err := s.db.Exec(
		`UPDATE sprints SET name = ?, goal = ?, state = ?, start_at = ?, end_at = ? WHERE id = ?`,
		sp.Name, sp.Goal, string(sp.State), ptrTimeToNull(sp.StartAt), ptrTimeToNull(sp.EndAt), sp.ID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.publish(core.Event{Type: core.EventSprintChanged, ProjectID: sp.ProjectID})
	return nil
}

// AddIssueToSprint assigns an issue to a sprint (or clears it when sprintID is
// nil, returning the issue to the backlog).
func (s *Store) AddIssueToSprint(issueID int64, sprintID *int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	var projectID int64
	if err := s.db.QueryRow(`SELECT project_id FROM issues WHERE id = ?`, issueID).Scan(&projectID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if _, err := s.db.Exec(
		`UPDATE issues SET sprint_id = ?, updated_at = ? WHERE id = ?`,
		ptrInt64ToNull(sprintID), fmtTime(s.now()), issueID,
	); err != nil {
		return err
	}
	s.publish(core.Event{Type: core.EventIssueUpdated, ProjectID: projectID, IssueID: issueID})
	return nil
}

// DeleteSprint removes a sprint; its issues are returned to the backlog via the
// ON DELETE SET NULL foreign key.
func (s *Store) DeleteSprint(id int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	var projectID int64
	if err := s.db.QueryRow(`SELECT project_id FROM sprints WHERE id = ?`, id).Scan(&projectID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM sprints WHERE id = ?`, id); err != nil {
		return err
	}
	s.publish(core.Event{Type: core.EventSprintChanged, ProjectID: projectID})
	return nil
}
