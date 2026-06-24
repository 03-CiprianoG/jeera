package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// AddComment appends an activity entry to an issue. Author defaults to "human"
// when empty; agent runs pass "run:<id>".
func (s *Store) AddComment(c core.Comment) (core.Comment, error) {
	if c.IssueID == 0 {
		return core.Comment{}, fmt.Errorf("%w: comment must belong to an issue", core.ErrInvalid)
	}
	if strings.TrimSpace(c.Body) == "" {
		return core.Comment{}, fmt.Errorf("%w: comment body is required", core.ErrInvalid)
	}
	if c.Author == "" {
		c.Author = "human"
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	var projectID int64
	if err := s.db.QueryRow(`SELECT project_id FROM issues WHERE id = ?`, c.IssueID).Scan(&projectID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return core.Comment{}, ErrNotFound
		}
		return core.Comment{}, err
	}

	c.CreatedAt = s.now()
	res, err := s.db.Exec(
		`INSERT INTO comments (issue_id, author, body, created_at) VALUES (?, ?, ?, ?)`,
		c.IssueID, c.Author, c.Body, fmtTime(c.CreatedAt),
	)
	if err != nil {
		return core.Comment{}, err
	}
	if c.ID, err = res.LastInsertId(); err != nil {
		return core.Comment{}, err
	}
	s.publish(core.Event{Type: core.EventCommentAdded, ProjectID: projectID, IssueID: c.IssueID})
	return c, nil
}

// ListComments returns an issue's comments oldest first.
func (s *Store) ListComments(issueID int64) ([]core.Comment, error) {
	rows, err := s.db.Query(
		`SELECT id, issue_id, author, body, created_at FROM comments WHERE issue_id = ? ORDER BY id`, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Comment
	for rows.Next() {
		var (
			c         core.Comment
			createdAt string
		)
		if err := rows.Scan(&c.ID, &c.IssueID, &c.Author, &c.Body, &createdAt); err != nil {
			return nil, err
		}
		if c.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
