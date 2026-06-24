package store

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// rowQuerier is satisfied by both *sql.DB and *sql.Tx, so reference checks can
// run either standalone or inside a transaction.
type rowQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
}

func projectOfStatus(q rowQuerier, statusID int64) (int64, error) {
	var pid int64
	err := q.QueryRow(`SELECT project_id FROM statuses WHERE id = ?`, statusID).Scan(&pid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return pid, err
}

func projectOfIssue(q rowQuerier, issueID int64) (int64, error) {
	var pid int64
	err := q.QueryRow(`SELECT project_id FROM issues WHERE id = ?`, issueID).Scan(&pid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return pid, err
}

func projectOfSprint(q rowQuerier, sprintID int64) (int64, error) {
	var pid int64
	err := q.QueryRow(`SELECT project_id FROM sprints WHERE id = ?`, sprintID).Scan(&pid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return pid, err
}

// requireSameProject confirms that the referenced row exists and belongs to the
// expected project, returning a core.ErrInvalid-wrapped error otherwise. The
// label names the reference in the error message (e.g. "status", "epic").
func requireSameProject(want int64, got int64, err error, label string, id int64) error {
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return fmt.Errorf("%w: %s %d does not exist", core.ErrInvalid, label, id)
		}
		return err
	}
	if got != want {
		return fmt.Errorf("%w: %s %d belongs to a different project", core.ErrInvalid, label, id)
	}
	return nil
}

// validateIssueRefs enforces a core integrity invariant the FKs cannot express:
// an issue's status, epic, parent and sprint must all live in the same project
// as the issue. Without this, an agent (via MCP) could file an issue into
// another project's column or hierarchy, silently corrupting every board view.
func validateIssueRefs(q rowQuerier, iss core.Issue) error {
	sp, err := projectOfStatus(q, iss.StatusID)
	if e := requireSameProject(iss.ProjectID, sp, err, "status", iss.StatusID); e != nil {
		return e
	}
	if iss.EpicID != nil {
		ep, err := projectOfIssue(q, *iss.EpicID)
		if e := requireSameProject(iss.ProjectID, ep, err, "epic", *iss.EpicID); e != nil {
			return e
		}
	}
	if iss.ParentID != nil {
		pp, err := projectOfIssue(q, *iss.ParentID)
		if e := requireSameProject(iss.ProjectID, pp, err, "parent", *iss.ParentID); e != nil {
			return e
		}
	}
	if iss.SprintID != nil {
		spp, err := projectOfSprint(q, *iss.SprintID)
		if e := requireSameProject(iss.ProjectID, spp, err, "sprint", *iss.SprintID); e != nil {
			return e
		}
	}
	return nil
}
