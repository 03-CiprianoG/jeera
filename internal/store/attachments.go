package store

import (
	"database/sql"
	"errors"

	"github.com/03-CiprianoG/jeera/internal/core"
)

const attachmentCols = `id, issue_id, filename, path, mime, size, created_at`

func scanAttachment(row interface{ Scan(...any) error }) (core.Attachment, error) {
	var (
		a         core.Attachment
		createdAt string
	)
	if err := row.Scan(&a.ID, &a.IssueID, &a.Filename, &a.Path, &a.MIME, &a.Size, &createdAt); err != nil {
		return core.Attachment{}, err
	}
	t, err := parseTime(createdAt)
	if err != nil {
		return core.Attachment{}, err
	}
	a.CreatedAt = t
	return a, nil
}

// CreateAttachment records a file/URL reference on an issue.
func (s *Store) CreateAttachment(a core.Attachment) (core.Attachment, error) {
	if a.Filename == "" && a.Path != "" {
		a.Filename = core.ClassifyAttachment(a.Path).Filename
	}
	if err := a.Validate(); err != nil {
		return core.Attachment{}, err
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = s.now()
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	res, err := s.db.Exec(
		`INSERT INTO attachments (issue_id, filename, path, mime, size, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		a.IssueID, a.Filename, a.Path, a.MIME, a.Size, fmtTime(a.CreatedAt),
	)
	if err != nil {
		return core.Attachment{}, err
	}
	if a.ID, err = res.LastInsertId(); err != nil {
		return core.Attachment{}, err
	}
	s.publish(core.Event{Type: core.EventIssueUpdated, IssueID: a.IssueID})
	return a, nil
}

// ListAttachments returns an issue's attachments, newest first.
func (s *Store) ListAttachments(issueID int64) ([]core.Attachment, error) {
	rows, err := s.db.Query(`SELECT `+attachmentCols+` FROM attachments WHERE issue_id = ? ORDER BY id DESC`, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Attachment
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteAttachment removes an attachment by ID.
func (s *Store) DeleteAttachment(id int64) error {
	issueID, _ := s.attachmentIssue(id)
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	res, err := s.db.Exec(`DELETE FROM attachments WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.publish(core.Event{Type: core.EventIssueUpdated, IssueID: issueID})
	return nil
}

func (s *Store) attachmentIssue(id int64) (int64, error) {
	var issueID int64
	err := s.db.QueryRow(`SELECT issue_id FROM attachments WHERE id = ?`, id).Scan(&issueID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return issueID, err
}
