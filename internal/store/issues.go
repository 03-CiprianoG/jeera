package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// issueCols is the issues column list in the order scanIssue expects, with the
// owning project's prefix appended so the derived Key can be built.
const issueSelect = `SELECT i.id, i.project_id, i.seq, i.type, i.title, i.description,
	i.status_id, i.priority, i.story_points,
	i.assignee_provider, i.assignee_model, i.assignee_effort,
	i.epic_id, i.sprint_id, i.parent_id, i.rank, i.worktree_on, i.settings,
	i.created_at, i.updated_at, p.key_prefix
	FROM issues i JOIN projects p ON p.id = i.project_id`

func scanIssue(row interface{ Scan(...any) error }) (core.Issue, error) {
	var (
		iss          core.Issue
		points       sql.NullInt64
		epicID       sql.NullInt64
		sprintID     sql.NullInt64
		parentID     sql.NullInt64
		worktreeOn   sql.NullInt64
		settingsJSON string
		createdAt    string
		updatedAt    string
		prefix       string
	)
	if err := row.Scan(
		&iss.ID, &iss.ProjectID, &iss.Seq, &iss.Type, &iss.Title, &iss.Description,
		&iss.StatusID, &iss.Priority, &points,
		&iss.Assignee.Provider, &iss.Assignee.Model, &iss.Assignee.Effort,
		&epicID, &sprintID, &parentID, &iss.Rank, &worktreeOn, &settingsJSON,
		&createdAt, &updatedAt, &prefix,
	); err != nil {
		return core.Issue{}, err
	}
	iss.StoryPoints = nullToPtrInt(points)
	iss.EpicID = nullToPtrInt64(epicID)
	iss.SprintID = nullToPtrInt64(sprintID)
	iss.ParentID = nullToPtrInt64(parentID)
	iss.WorktreeOn = nullToPtrBool(worktreeOn)
	if err := json.Unmarshal([]byte(settingsJSON), &iss.Settings); err != nil {
		return core.Issue{}, fmt.Errorf("store: unmarshal issue settings: %w", err)
	}
	var err error
	if iss.CreatedAt, err = parseTime(createdAt); err != nil {
		return core.Issue{}, err
	}
	if iss.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return core.Issue{}, err
	}
	iss.Key = core.FormatKey(prefix, iss.Seq)
	return iss, nil
}

// CreateIssue allocates the next per-project sequence number, derives the key,
// fills in sensible defaults (task/medium, the first board column, a rank), and
// inserts the issue. The sequence allocation and insert run in one transaction
// so keys are gap-tolerant but never duplicated.
func (s *Store) CreateIssue(iss core.Issue) (core.Issue, error) {
	if iss.Type == "" {
		iss.Type = core.TypeTask
	}
	if iss.Priority == "" {
		iss.Priority = core.PriorityMedium
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return core.Issue{}, err
	}
	defer tx.Rollback()

	// Default to the project's first column when none was supplied.
	if iss.StatusID == 0 {
		if err := tx.QueryRow(
			`SELECT id FROM statuses WHERE project_id = ? ORDER BY position, id LIMIT 1`,
			iss.ProjectID,
		).Scan(&iss.StatusID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return core.Issue{}, fmt.Errorf("%w: project %d has no statuses", core.ErrInvalid, iss.ProjectID)
			}
			return core.Issue{}, err
		}
	}

	now := s.now()
	iss.CreatedAt = now
	iss.UpdatedAt = now
	if iss.Rank == "" {
		iss.Rank = fmtTime(now)
	}
	if err := iss.Validate(); err != nil {
		return core.Issue{}, err
	}

	// Allocate the sequence number atomically within the transaction.
	if _, err := tx.Exec(`UPDATE projects SET seq_counter = seq_counter + 1 WHERE id = ?`, iss.ProjectID); err != nil {
		return core.Issue{}, err
	}
	if err := tx.QueryRow(`SELECT seq_counter FROM projects WHERE id = ?`, iss.ProjectID).Scan(&iss.Seq); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return core.Issue{}, ErrNotFound
		}
		return core.Issue{}, err
	}

	settingsJSON, err := json.Marshal(iss.Settings)
	if err != nil {
		return core.Issue{}, err
	}
	res, err := tx.Exec(
		`INSERT INTO issues (project_id, seq, type, title, description, status_id, priority,
			story_points, assignee_provider, assignee_model, assignee_effort,
			epic_id, sprint_id, parent_id, rank, worktree_on, settings, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		iss.ProjectID, iss.Seq, string(iss.Type), iss.Title, iss.Description, iss.StatusID, string(iss.Priority),
		ptrIntToNull(iss.StoryPoints), string(iss.Assignee.Provider), iss.Assignee.Model, string(iss.Assignee.Effort),
		ptrInt64ToNull(iss.EpicID), ptrInt64ToNull(iss.SprintID), ptrInt64ToNull(iss.ParentID),
		iss.Rank, ptrBoolToNull(iss.WorktreeOn), string(settingsJSON), fmtTime(iss.CreatedAt), fmtTime(iss.UpdatedAt),
	)
	if err != nil {
		return core.Issue{}, fmt.Errorf("store: insert issue: %w", err)
	}
	if iss.ID, err = res.LastInsertId(); err != nil {
		return core.Issue{}, err
	}
	if err := tx.Commit(); err != nil {
		return core.Issue{}, err
	}

	p, err := s.GetProject(iss.ProjectID)
	if err != nil {
		return core.Issue{}, err
	}
	iss.Key = core.FormatKey(p.KeyPrefix, iss.Seq)
	s.publish(core.Event{Type: core.EventIssueCreated, ProjectID: iss.ProjectID, IssueID: iss.ID})
	return iss, nil
}

// GetIssue returns an issue by its database ID.
func (s *Store) GetIssue(id int64) (core.Issue, error) {
	row := s.db.QueryRow(issueSelect+` WHERE i.id = ?`, id)
	iss, err := scanIssue(row)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Issue{}, ErrNotFound
	}
	return iss, err
}

// GetIssueByKey returns an issue by its human key, e.g. "JEE-12".
func (s *Store) GetIssueByKey(key string) (core.Issue, error) {
	prefix, seq, err := core.ParseKey(key)
	if err != nil {
		return core.Issue{}, fmt.Errorf("%w: %v", core.ErrInvalid, err)
	}
	row := s.db.QueryRow(issueSelect+` WHERE p.key_prefix = ? AND i.seq = ?`, prefix, seq)
	iss, err := scanIssue(row)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Issue{}, ErrNotFound
	}
	return iss, err
}

// IssueFilter narrows ListIssues. Zero-valued fields are ignored, so an empty
// filter lists every issue. SprintID/EpicID match a specific parent; the
// Unsprinted/NoEpic flags match the absence of one (for the backlog and
// orphan views).
type IssueFilter struct {
	ProjectID  int64
	StatusID   int64
	Type       core.IssueType
	SprintID   *int64
	EpicID     *int64
	ParentID   *int64
	Unsprinted bool
	Text       string
}

// ListIssues returns issues matching the filter, ordered by rank then key.
func (s *Store) ListIssues(f IssueFilter) ([]core.Issue, error) {
	var where []string
	var args []any
	if f.ProjectID != 0 {
		where = append(where, "i.project_id = ?")
		args = append(args, f.ProjectID)
	}
	if f.StatusID != 0 {
		where = append(where, "i.status_id = ?")
		args = append(args, f.StatusID)
	}
	if f.Type != "" {
		where = append(where, "i.type = ?")
		args = append(args, string(f.Type))
	}
	if f.SprintID != nil {
		where = append(where, "i.sprint_id = ?")
		args = append(args, *f.SprintID)
	}
	if f.Unsprinted {
		where = append(where, "i.sprint_id IS NULL")
	}
	if f.EpicID != nil {
		where = append(where, "i.epic_id = ?")
		args = append(args, *f.EpicID)
	}
	if f.ParentID != nil {
		where = append(where, "i.parent_id = ?")
		args = append(args, *f.ParentID)
	}
	if t := strings.TrimSpace(f.Text); t != "" {
		where = append(where, "(i.title LIKE ? OR i.description LIKE ?)")
		like := "%" + t + "%"
		args = append(args, like, like)
	}

	q := issueSelect
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY i.rank, i.seq"

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Issue
	for rows.Next() {
		iss, err := scanIssue(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, iss)
	}
	return out, rows.Err()
}

// UpdateIssue persists all mutable fields of an issue and bumps UpdatedAt. Seq,
// Key, ProjectID and CreatedAt are immutable and ignored.
func (s *Store) UpdateIssue(iss core.Issue) error {
	if iss.Type == "" {
		iss.Type = core.TypeTask
	}
	if iss.Priority == "" {
		iss.Priority = core.PriorityMedium
	}
	if err := iss.Validate(); err != nil {
		return err
	}
	settingsJSON, err := json.Marshal(iss.Settings)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	iss.UpdatedAt = s.now()
	res, err := s.db.Exec(
		`UPDATE issues SET type = ?, title = ?, description = ?, status_id = ?, priority = ?,
			story_points = ?, assignee_provider = ?, assignee_model = ?, assignee_effort = ?,
			epic_id = ?, sprint_id = ?, parent_id = ?, rank = ?, worktree_on = ?, settings = ?, updated_at = ?
		 WHERE id = ?`,
		string(iss.Type), iss.Title, iss.Description, iss.StatusID, string(iss.Priority),
		ptrIntToNull(iss.StoryPoints), string(iss.Assignee.Provider), iss.Assignee.Model, string(iss.Assignee.Effort),
		ptrInt64ToNull(iss.EpicID), ptrInt64ToNull(iss.SprintID), ptrInt64ToNull(iss.ParentID),
		iss.Rank, ptrBoolToNull(iss.WorktreeOn), string(settingsJSON), fmtTime(iss.UpdatedAt), iss.ID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.publish(core.Event{Type: core.EventIssueUpdated, ProjectID: iss.ProjectID, IssueID: iss.ID})
	return nil
}

// TransitionIssue moves an issue to a different status (board column). The
// target status must belong to the same project as the issue.
func (s *Store) TransitionIssue(issueID, statusID int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	var issueProject, statusProject int64
	if err := s.db.QueryRow(`SELECT project_id FROM issues WHERE id = ?`, issueID).Scan(&issueProject); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if err := s.db.QueryRow(`SELECT project_id FROM statuses WHERE id = ?`, statusID).Scan(&statusProject); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: status %d does not exist", core.ErrInvalid, statusID)
		}
		return err
	}
	if issueProject != statusProject {
		return fmt.Errorf("%w: status %d belongs to a different project", core.ErrInvalid, statusID)
	}
	if _, err := s.db.Exec(
		`UPDATE issues SET status_id = ?, updated_at = ? WHERE id = ?`,
		statusID, fmtTime(s.now()), issueID,
	); err != nil {
		return err
	}
	s.publish(core.Event{Type: core.EventIssueUpdated, ProjectID: issueProject, IssueID: issueID})
	return nil
}

// StatusByName resolves a status by its display name within a project (used by
// the MCP transition_issue tool, which takes a status name).
func (s *Store) StatusByName(projectID int64, name string) (core.Status, error) {
	var st core.Status
	err := s.db.QueryRow(
		`SELECT id, project_id, name, category, position FROM statuses
		 WHERE project_id = ? AND name = ? COLLATE NOCASE`, projectID, name,
	).Scan(&st.ID, &st.ProjectID, &st.Name, &st.Category, &st.Position)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Status{}, ErrNotFound
	}
	return st, err
}

// DeleteIssue removes an issue and its dependent rows (comments, links, runs…)
// by cascade.
func (s *Store) DeleteIssue(id int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	var projectID int64
	if err := s.db.QueryRow(`SELECT project_id FROM issues WHERE id = ?`, id).Scan(&projectID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM issues WHERE id = ?`, id); err != nil {
		return err
	}
	s.publish(core.Event{Type: core.EventIssueDeleted, ProjectID: projectID, IssueID: id})
	return nil
}
