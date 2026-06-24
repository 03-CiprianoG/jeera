package store

import (
	"database/sql"
	"errors"

	"github.com/03-CiprianoG/jeera/internal/core"
)

const scheduleCols = `id, issue_id, cron_spec, with_children, enabled, job_uuid, next_run`

func scanSchedule(row interface{ Scan(...any) error }) (core.Schedule, error) {
	var (
		s       core.Schedule
		nextRun sql.NullString
	)
	if err := row.Scan(
		&s.ID, &s.IssueID, &s.CronSpec, &s.WithChildren, &s.Enabled, &s.JobUUID, &nextRun,
	); err != nil {
		return core.Schedule{}, err
	}
	var err error
	if s.NextRun, err = nullToPtrTime(nextRun); err != nil {
		return core.Schedule{}, err
	}
	return s, nil
}

// CreateSchedule inserts a schedule and returns it with its ID populated.
func (s *Store) CreateSchedule(sc core.Schedule) (core.Schedule, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	res, err := s.db.Exec(
		`INSERT INTO schedules (issue_id, cron_spec, with_children, enabled, job_uuid, next_run)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		sc.IssueID, sc.CronSpec, sc.WithChildren, sc.Enabled, sc.JobUUID, ptrTimeToNull(sc.NextRun),
	)
	if err != nil {
		return core.Schedule{}, err
	}
	if sc.ID, err = res.LastInsertId(); err != nil {
		return core.Schedule{}, err
	}
	s.publish(core.Event{Type: core.EventScheduleChanged, IssueID: sc.IssueID})
	return sc, nil
}

// UpdateSchedule persists a schedule's mutable fields (cron, enabled, job binding,
// next run).
func (s *Store) UpdateSchedule(sc core.Schedule) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	res, err := s.db.Exec(
		`UPDATE schedules SET cron_spec = ?, with_children = ?, enabled = ?, job_uuid = ?, next_run = ?
		 WHERE id = ?`,
		sc.CronSpec, sc.WithChildren, sc.Enabled, sc.JobUUID, ptrTimeToNull(sc.NextRun), sc.ID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.publish(core.Event{Type: core.EventScheduleChanged, IssueID: sc.IssueID})
	return nil
}

// GetSchedule returns a schedule by ID.
func (s *Store) GetSchedule(id int64) (core.Schedule, error) {
	row := s.db.QueryRow(`SELECT `+scheduleCols+` FROM schedules WHERE id = ?`, id)
	sc, err := scanSchedule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Schedule{}, ErrNotFound
	}
	return sc, err
}

// DeleteSchedule removes a schedule.
func (s *Store) DeleteSchedule(id int64) error {
	issueID, _ := s.scheduleIssue(id)
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	res, err := s.db.Exec(`DELETE FROM schedules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.publish(core.Event{Type: core.EventScheduleChanged, IssueID: issueID})
	return nil
}

// ListSchedules returns an issue's schedules, newest first.
func (s *Store) ListSchedules(issueID int64) ([]core.Schedule, error) {
	return s.querySchedules(`SELECT `+scheduleCols+` FROM schedules WHERE issue_id = ? ORDER BY id DESC`, issueID)
}

// ListEnabledSchedules returns every enabled schedule across all issues — the set
// the scheduler registers on boot.
func (s *Store) ListEnabledSchedules() ([]core.Schedule, error) {
	return s.querySchedules(`SELECT ` + scheduleCols + ` FROM schedules WHERE enabled = 1 ORDER BY id`)
}

func (s *Store) scheduleIssue(id int64) (int64, error) {
	var issueID int64
	err := s.db.QueryRow(`SELECT issue_id FROM schedules WHERE id = ?`, id).Scan(&issueID)
	return issueID, err
}

func (s *Store) querySchedules(q string, args ...any) ([]core.Schedule, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Schedule
	for rows.Next() {
		sc, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}
