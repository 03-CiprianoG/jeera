package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// CreateTag adds a project-scoped label. Tag names are unique per project.
func (s *Store) CreateTag(t core.Tag) (core.Tag, error) {
	if t.ProjectID == 0 {
		return core.Tag{}, fmt.Errorf("%w: tag must belong to a project", core.ErrInvalid)
	}
	if strings.TrimSpace(t.Name) == "" {
		return core.Tag{}, fmt.Errorf("%w: tag name is required", core.ErrInvalid)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	res, err := s.db.Exec(
		`INSERT INTO tags (project_id, name, color) VALUES (?, ?, ?)`,
		t.ProjectID, t.Name, t.Color,
	)
	if err != nil {
		return core.Tag{}, err
	}
	if t.ID, err = res.LastInsertId(); err != nil {
		return core.Tag{}, err
	}
	s.publish(core.Event{Type: core.EventProjectChanged, ProjectID: t.ProjectID})
	return t, nil
}

// ListTags returns a project's tags ordered by name.
func (s *Store) ListTags(projectID int64) ([]core.Tag, error) {
	rows, err := s.db.Query(`SELECT id, project_id, name, color FROM tags WHERE project_id = ? ORDER BY name`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Tag
	for rows.Next() {
		var t core.Tag
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Name, &t.Color); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TagIssue attaches a tag to an issue. It is idempotent: re-tagging is a no-op.
func (s *Store) TagIssue(issueID, tagID int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := s.db.Exec(
		`INSERT OR IGNORE INTO issue_tags (issue_id, tag_id) VALUES (?, ?)`, issueID, tagID,
	); err != nil {
		return err
	}
	s.publishIssue(issueID, core.EventIssueUpdated)
	return nil
}

// UntagIssue detaches a tag from an issue.
func (s *Store) UntagIssue(issueID, tagID int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := s.db.Exec(`DELETE FROM issue_tags WHERE issue_id = ? AND tag_id = ?`, issueID, tagID); err != nil {
		return err
	}
	s.publishIssue(issueID, core.EventIssueUpdated)
	return nil
}

// ListIssueTags returns the tags attached to an issue.
func (s *Store) ListIssueTags(issueID int64) ([]core.Tag, error) {
	rows, err := s.db.Query(
		`SELECT t.id, t.project_id, t.name, t.color FROM tags t
		 JOIN issue_tags it ON it.tag_id = t.id
		 WHERE it.issue_id = ? ORDER BY t.name`, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Tag
	for rows.Next() {
		var t core.Tag
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Name, &t.Color); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// publishIssue looks up an issue's project and emits the given event. It assumes
// the caller already holds writeMu.
func (s *Store) publishIssue(issueID int64, typ core.EventType) {
	var projectID int64
	if err := s.db.QueryRow(`SELECT project_id FROM issues WHERE id = ?`, issueID).Scan(&projectID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return
	}
	s.publish(core.Event{Type: typ, ProjectID: projectID, IssueID: issueID})
}
