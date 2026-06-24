package store

import (
	"github.com/03-CiprianoG/jeera/internal/core"
)

// CreateLink records a directional relationship between two issues. The two
// issues must belong to the same project; duplicates (same source, target and
// type) are ignored.
func (s *Store) CreateLink(l core.IssueLink) (core.IssueLink, error) {
	if l.ProjectID == 0 {
		// Derive the project from the source issue when the caller omits it.
		var projectID int64
		if err := s.db.QueryRow(`SELECT project_id FROM issues WHERE id = ?`, l.SourceID).Scan(&projectID); err == nil {
			l.ProjectID = projectID
		}
	}
	if err := l.Validate(); err != nil {
		return core.IssueLink{}, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO issue_links (project_id, source_id, target_id, type) VALUES (?, ?, ?, ?)`,
		l.ProjectID, l.SourceID, l.TargetID, string(l.Type),
	)
	if err != nil {
		return core.IssueLink{}, err
	}
	if l.ID, err = res.LastInsertId(); err != nil {
		return core.IssueLink{}, err
	}
	s.publish(core.Event{Type: core.EventIssueUpdated, ProjectID: l.ProjectID, IssueID: l.SourceID})
	return l, nil
}

// LinkedIssue pairs a related issue with the relationship as seen from the
// perspective of the queried issue.
type LinkedIssue struct {
	LinkID int64
	Type   core.LinkType
	Issue  core.Issue
}

// ListLinks returns every issue related to issueID, presenting each edge from
// issueID's perspective: an edge stored as "A blocks B" appears as "blocks"
// when queried from A and as "blocked_by" when queried from B.
func (s *Store) ListLinks(issueID int64) ([]LinkedIssue, error) {
	rows, err := s.db.Query(
		`SELECT l.id, l.type, l.source_id, l.target_id FROM issue_links l
		 WHERE l.source_id = ? OR l.target_id = ? ORDER BY l.id`, issueID, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type edge struct {
		linkID         int64
		typ            core.LinkType
		source, target int64
	}
	var edges []edge
	for rows.Next() {
		var e edge
		if err := rows.Scan(&e.linkID, &e.typ, &e.source, &e.target); err != nil {
			return nil, err
		}
		edges = append(edges, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]LinkedIssue, 0, len(edges))
	for _, e := range edges {
		otherID := e.target
		typ := e.typ
		if e.source != issueID {
			otherID = e.source
			typ = e.typ.Inverse()
		}
		other, err := s.GetIssue(otherID)
		if err != nil {
			return nil, err
		}
		out = append(out, LinkedIssue{LinkID: e.linkID, Type: typ, Issue: other})
	}
	return out, nil
}

// DeleteLink removes a relationship edge by its ID.
func (s *Store) DeleteLink(linkID int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	res, err := s.db.Exec(`DELETE FROM issue_links WHERE id = ?`, linkID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
