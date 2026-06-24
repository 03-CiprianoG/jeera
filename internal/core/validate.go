package core

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalid is the sentinel wrapped by every validation error in this package,
// so callers (the store, the MCP layer) can classify a failure as a bad request
// with errors.Is(err, core.ErrInvalid).
var ErrInvalid = errors.New("core: invalid")

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Validate checks the project's user-supplied fields. It does not check
// database identity (ID) or server-assigned fields.
func (p Project) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return invalidf("project name is required")
	}
	if !ValidPrefix(p.KeyPrefix) {
		return invalidf("project key prefix %q must be 2-10 letters/digits starting with a letter", p.KeyPrefix)
	}
	if strings.TrimSpace(p.RepoPath) == "" {
		return invalidf("project repo path is required")
	}
	if p.Defaults.Provider != "" && !p.Defaults.Provider.Valid() {
		return invalidf("unknown provider %q", p.Defaults.Provider)
	}
	if p.Defaults.Effort != "" && !p.Defaults.Effort.Valid() {
		return invalidf("unknown effort %q", p.Defaults.Effort)
	}
	return nil
}

// Validate checks an issue's user-supplied fields. StatusID is required because
// every issue lives in a column; the store assigns Seq/Key, so they are not
// checked here.
func (i Issue) Validate() error {
	if i.ProjectID == 0 {
		return invalidf("issue must belong to a project")
	}
	if strings.TrimSpace(i.Title) == "" {
		return invalidf("issue title is required")
	}
	if !i.Type.Valid() {
		return invalidf("unknown issue type %q", i.Type)
	}
	if !i.Priority.Valid() {
		return invalidf("unknown priority %q", i.Priority)
	}
	if i.StatusID == 0 {
		return invalidf("issue must have a status")
	}
	if i.StoryPoints != nil && *i.StoryPoints < 0 {
		return invalidf("story points cannot be negative")
	}
	if err := i.Assignee.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate checks an assignee. A zero assignee (no model assigned) is valid; a
// partially-filled one is not.
func (a Assignee) Validate() error {
	if a.IsZero() {
		return nil
	}
	if !a.Provider.Valid() {
		return invalidf("unknown provider %q", a.Provider)
	}
	if strings.TrimSpace(a.Model) == "" {
		return invalidf("assignee model is required when an assignee is set")
	}
	if a.Effort != "" && !a.Effort.Valid() {
		return invalidf("unknown effort %q", a.Effort)
	}
	return nil
}

// Validate checks a status definition.
func (s Status) Validate() error {
	if s.ProjectID == 0 {
		return invalidf("status must belong to a project")
	}
	if strings.TrimSpace(s.Name) == "" {
		return invalidf("status name is required")
	}
	if !s.Category.Valid() {
		return invalidf("unknown status category %q", s.Category)
	}
	return nil
}

// Validate checks a sprint definition.
func (s Sprint) Validate() error {
	if s.ProjectID == 0 {
		return invalidf("sprint must belong to a project")
	}
	if strings.TrimSpace(s.Name) == "" {
		return invalidf("sprint name is required")
	}
	if s.State != "" && !s.State.Valid() {
		return invalidf("unknown sprint state %q", s.State)
	}
	if s.StartAt != nil && s.EndAt != nil && s.EndAt.Before(*s.StartAt) {
		return invalidf("sprint end cannot be before its start")
	}
	return nil
}

// Validate checks an issue link.
func (l IssueLink) Validate() error {
	if l.ProjectID == 0 {
		return invalidf("link must belong to a project")
	}
	if l.SourceID == 0 || l.TargetID == 0 {
		return invalidf("link needs both a source and a target issue")
	}
	if l.SourceID == l.TargetID {
		return invalidf("an issue cannot be linked to itself")
	}
	if !l.Type.Valid() {
		return invalidf("unknown link type %q", l.Type)
	}
	return nil
}
