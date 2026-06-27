package store

import (
	"database/sql"
	"errors"
	"fmt"

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
	// A project may have only one active sprint at a time. Creating one directly
	// in the active state (tests, a future MCP create_sprint) must respect that
	// before the partial unique index rejects it with a raw constraint error; the
	// TUI only ever creates future sprints, so this never trips for it.
	if sp.State == core.SprintActive {
		var active int64
		if err := s.db.QueryRow(
			`SELECT COUNT(*) FROM sprints WHERE project_id = ? AND state = ?`,
			sp.ProjectID, string(core.SprintActive),
		).Scan(&active); err != nil {
			return core.Sprint{}, err
		}
		if active > 0 {
			return core.Sprint{}, fmt.Errorf("%w: a sprint is already active in this project", core.ErrInvalid)
		}
	}
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

// ActiveSprint returns the project's running sprint, if any. The boolean is
// false (with a nil error) when no sprint is active — the normal state while a
// project is being planned or sits between sprints. At most one sprint per
// project is active at a time (StartSprint enforces it, backed by a partial
// unique index), so the result is unambiguous.
func (s *Store) ActiveSprint(projectID int64) (core.Sprint, bool, error) {
	row := s.db.QueryRow(
		`SELECT `+sprintCols+` FROM sprints WHERE project_id = ? AND state = ? LIMIT 1`,
		projectID, string(core.SprintActive),
	)
	sp, err := scanSprint(row)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Sprint{}, false, nil
	}
	if err != nil {
		return core.Sprint{}, false, err
	}
	return sp, true, nil
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

// StartSprint makes a sprint its project's active one: it sets the state to
// active and stamps StartAt if it was unset. A project may have only one active
// sprint at a time — the SCRUM board scopes to "the" active sprint, which must
// be singular — so it fails with core.ErrInvalid when another is already
// running. It deliberately does not require a particular prior state, so a
// reopened (future) sprint can be started again.
func (s *Store) StartSprint(id int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	var projectID int64
	if err := s.db.QueryRow(`SELECT project_id FROM sprints WHERE id = ?`, id).Scan(&projectID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	// One active sprint per project: surface a friendly error before the partial
	// unique index would reject the write with a raw constraint failure.
	var others int64
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM sprints WHERE project_id = ? AND state = ? AND id <> ?`,
		projectID, string(core.SprintActive), id,
	).Scan(&others); err != nil {
		return err
	}
	if others > 0 {
		return fmt.Errorf("%w: another sprint is already active — finish it first", core.ErrInvalid)
	}

	if _, err := s.db.Exec(
		`UPDATE sprints SET state = ?, start_at = COALESCE(start_at, ?) WHERE id = ?`,
		string(core.SprintActive), fmtTime(s.now()), id,
	); err != nil {
		return err
	}
	s.publish(core.Event{Type: core.EventSprintChanged, ProjectID: projectID})
	return nil
}

// CompleteSprint closes a sprint and rolls its unfinished work back to the
// backlog: every issue in the sprint whose status is not in the done category
// has its sprint cleared, while done issues stay attached so the completed
// sprint keeps the record of what shipped. The rollover and the state change
// commit in one transaction, so the board can never show a sprint that is
// "completed" yet still holds live issues.
func (s *Store) CompleteSprint(id int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	var projectID int64
	if err := s.db.QueryRow(`SELECT project_id FROM sprints WHERE id = ?`, id).Scan(&projectID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := fmtTime(s.now())
	// Return incomplete issues (any status not in the done category) to the
	// backlog. Statuses are project-scoped, so the subquery is bounded to this
	// project's columns.
	if _, err := tx.Exec(
		`UPDATE issues SET sprint_id = NULL, updated_at = ?
		   WHERE sprint_id = ? AND status_id IN (
		     SELECT id FROM statuses WHERE project_id = ? AND category <> ?)`,
		now, id, projectID, string(core.CategoryDone),
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE sprints SET state = ?, end_at = COALESCE(end_at, ?) WHERE id = ?`,
		string(core.SprintCompleted), now, id,
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// A single event suffices: reload() re-reads the whole board and views on any
	// change, the same way DeleteSprint publishes one event despite nulling many
	// issues' sprint_id via cascade.
	s.publish(core.Event{Type: core.EventSprintChanged, ProjectID: projectID})
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
	// A sprint and the issue assigned to it must belong to the same project.
	if sprintID != nil {
		sp, err := projectOfSprint(s.db, *sprintID)
		if e := requireSameProject(projectID, sp, err, "sprint", *sprintID); e != nil {
			return e
		}
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
