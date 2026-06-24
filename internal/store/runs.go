package store

import (
	"database/sql"
	"errors"

	"github.com/03-CiprianoG/jeera/internal/core"
)

const runCols = `id, issue_id, version, parent_run_id, provider, model, effort, session_id,
	worktree_path, branch, status, permission_mode, started_at, ended_at, exit_code, log_path`

func scanRun(row interface{ Scan(...any) error }) (core.Run, error) {
	var (
		r         core.Run
		parentRun sql.NullInt64
		startedAt sql.NullString
		endedAt   sql.NullString
		exitCode  sql.NullInt64
	)
	if err := row.Scan(
		&r.ID, &r.IssueID, &r.Version, &parentRun, &r.Provider, &r.Model, &r.Effort, &r.SessionID,
		&r.WorktreePath, &r.Branch, &r.Status, &r.PermissionMode, &startedAt, &endedAt, &exitCode, &r.LogPath,
	); err != nil {
		return core.Run{}, err
	}
	r.ParentRunID = nullToPtrInt64(parentRun)
	r.ExitCode = nullToPtrInt(exitCode)
	var err error
	if r.StartedAt, err = nullToPtrTime(startedAt); err != nil {
		return core.Run{}, err
	}
	if r.EndedAt, err = nullToPtrTime(endedAt); err != nil {
		return core.Run{}, err
	}
	return r, nil
}

// NextRunVersion returns the version number the next run of an issue should use
// (1 for the first run).
func (s *Store) NextRunVersion(issueID int64) (int, error) {
	var v int
	err := s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) + 1 FROM runs WHERE issue_id = ?`, issueID).Scan(&v)
	return v, err
}

// CreateRun inserts a run and returns it with its ID populated.
func (s *Store) CreateRun(r core.Run) (core.Run, error) {
	if r.Version == 0 {
		v, err := s.NextRunVersion(r.IssueID)
		if err != nil {
			return core.Run{}, err
		}
		r.Version = v
	}
	if r.Status == "" {
		r.Status = core.RunQueued
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	res, err := s.db.Exec(
		`INSERT INTO runs (issue_id, version, parent_run_id, provider, model, effort, session_id,
			worktree_path, branch, status, permission_mode, started_at, ended_at, exit_code, log_path, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.IssueID, r.Version, ptrInt64ToNull(r.ParentRunID), string(r.Provider), r.Model, string(r.Effort), r.SessionID,
		r.WorktreePath, r.Branch, string(r.Status), r.PermissionMode,
		ptrTimeToNull(r.StartedAt), ptrTimeToNull(r.EndedAt), ptrIntToNull(r.ExitCode), r.LogPath, fmtTime(s.now()),
	)
	if err != nil {
		return core.Run{}, err
	}
	if r.ID, err = res.LastInsertId(); err != nil {
		return core.Run{}, err
	}
	s.publish(core.Event{Type: core.EventRunChanged, IssueID: r.IssueID, RunID: r.ID})
	return r, nil
}

// UpdateRun persists a run's mutable fields (status, session, timing, exit code…).
func (s *Store) UpdateRun(r core.Run) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	res, err := s.db.Exec(
		`UPDATE runs SET session_id = ?, worktree_path = ?, branch = ?, status = ?,
			started_at = ?, ended_at = ?, exit_code = ?, log_path = ? WHERE id = ?`,
		r.SessionID, r.WorktreePath, r.Branch, string(r.Status),
		ptrTimeToNull(r.StartedAt), ptrTimeToNull(r.EndedAt), ptrIntToNull(r.ExitCode), r.LogPath, r.ID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.publish(core.Event{Type: core.EventRunChanged, IssueID: r.IssueID, RunID: r.ID})
	return nil
}

// GetRun returns a run by ID.
func (s *Store) GetRun(id int64) (core.Run, error) {
	row := s.db.QueryRow(`SELECT `+runCols+` FROM runs WHERE id = ?`, id)
	r, err := scanRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Run{}, ErrNotFound
	}
	return r, err
}

// ListRuns returns an issue's runs, newest version first.
func (s *Store) ListRuns(issueID int64) ([]core.Run, error) {
	return s.queryRuns(`SELECT `+runCols+` FROM runs WHERE issue_id = ? ORDER BY version DESC, id DESC`, issueID)
}

// ListActiveRuns returns all runs that are queued or running, across projects.
func (s *Store) ListActiveRuns() ([]core.Run, error) {
	return s.queryRuns(`SELECT ` + runCols + ` FROM runs WHERE status IN ('queued','running') ORDER BY id DESC`)
}

// ListRecentRuns returns the most recent runs across all issues.
func (s *Store) ListRecentRuns(limit int) ([]core.Run, error) {
	return s.queryRuns(`SELECT `+runCols+` FROM runs ORDER BY id DESC LIMIT ?`, limit)
}

func (s *Store) queryRuns(q string, args ...any) ([]core.Run, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
