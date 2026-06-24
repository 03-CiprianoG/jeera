package store

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// CreateLink records a directional relationship between two issues. The two
// issues belong to the same project (derived from the source when omitted);
// duplicates (same source, target and type) are idempotent no-ops that return
// the existing edge.
func (s *Store) CreateLink(l core.IssueLink) (core.IssueLink, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	// Derive the project from the source issue when the caller omits it, and
	// surface a clear not-found rather than a misleading validation error.
	if l.ProjectID == 0 && l.SourceID != 0 {
		pid, err := projectOfIssue(s.db, l.SourceID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return core.IssueLink{}, fmt.Errorf("%w: source issue %d does not exist", core.ErrInvalid, l.SourceID)
			}
			return core.IssueLink{}, err
		}
		l.ProjectID = pid
	}
	if err := l.Validate(); err != nil {
		return core.IssueLink{}, err
	}

	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO issue_links (project_id, source_id, target_id, type) VALUES (?, ?, ?, ?)`,
		l.ProjectID, l.SourceID, l.TargetID, string(l.Type),
	)
	if err != nil {
		return core.IssueLink{}, err
	}
	// INSERT OR IGNORE does not update last_insert_rowid on a conflict, so when
	// the edge already existed (0 rows affected) re-select its id instead of
	// trusting LastInsertId, which would return a stale rowid from a prior write.
	if n, _ := res.RowsAffected(); n == 0 {
		if err := s.db.QueryRow(
			`SELECT id FROM issue_links WHERE source_id = ? AND target_id = ? AND type = ?`,
			l.SourceID, l.TargetID, string(l.Type),
		).Scan(&l.ID); err != nil {
			return core.IssueLink{}, err
		}
	} else if l.ID, err = res.LastInsertId(); err != nil {
		return core.IssueLink{}, err
	}

	// Both endpoints' detail views change (one side shows the inverse), so
	// notify both issues.
	s.publish(core.Event{Type: core.EventIssueUpdated, ProjectID: l.ProjectID, IssueID: l.SourceID})
	s.publish(core.Event{Type: core.EventIssueUpdated, ProjectID: l.ProjectID, IssueID: l.TargetID})
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
			// The linked issue was concurrently deleted between reading the
			// edges and fetching it; skip it rather than failing the whole call.
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		out = append(out, LinkedIssue{LinkID: e.linkID, Type: typ, Issue: other})
	}
	return out, nil
}

// DeleteLink removes a relationship edge by its ID and notifies both endpoints
// so either issue's detail view refreshes.
func (s *Store) DeleteLink(linkID int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	var projectID, sourceID, targetID int64
	err := s.db.QueryRow(
		`SELECT project_id, source_id, target_id FROM issue_links WHERE id = ?`, linkID,
	).Scan(&projectID, &sourceID, &targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM issue_links WHERE id = ?`, linkID); err != nil {
		return err
	}
	s.publish(core.Event{Type: core.EventIssueUpdated, ProjectID: projectID, IssueID: sourceID})
	s.publish(core.Event{Type: core.EventIssueUpdated, ProjectID: projectID, IssueID: targetID})
	return nil
}
