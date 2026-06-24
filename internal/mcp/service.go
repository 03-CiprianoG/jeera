// Package mcp exposes Jeera's shared store to AI agents over the Model Context
// Protocol. It registers a set of typed tools on an mcp.Server and serves them
// over local HTTP (Streamable HTTP), so an agent reads and writes the very same
// issues the human sees in the TUI.
package mcp

import (
	"fmt"
	"strings"
	"time"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/store"
)

// Service is the bridge between MCP tool calls and the store. Every tool handler
// is a method on Service, so they all share one store and one consistent
// mapping to the agent-facing DTOs below.
type Service struct {
	store *store.Store
}

// NewService wraps a store for MCP access.
func NewService(st *store.Store) *Service { return &Service{store: st} }

// --- agent-facing DTOs -------------------------------------------------------
// These present the domain model to agents with stable, documented, key-centric
// field names (agents reference issues by key like "JEE-12", and statuses,
// epics, sprints and tags by name) rather than the internal numeric IDs.

type ProjectDTO struct {
	KeyPrefix string      `json:"key_prefix" jsonschema:"the project key prefix, e.g. JEE"`
	Name      string      `json:"name"`
	RepoPath  string      `json:"repo_path" jsonschema:"absolute path to the project's git repository"`
	Statuses  []StatusDTO `json:"statuses,omitempty" jsonschema:"the project's board columns"`
}

type StatusDTO struct {
	Name     string `json:"name"`
	Category string `json:"category" jsonschema:"todo, inprogress, or done"`
}

type AssigneeDTO struct {
	Provider string `json:"provider" jsonschema:"claude or codex"`
	Model    string `json:"model"`
	Effort   string `json:"effort,omitempty" jsonschema:"reasoning effort level"`
}

type IssueDTO struct {
	Key            string       `json:"key" jsonschema:"issue key, e.g. JEE-12"`
	Title          string       `json:"title"`
	Type           string       `json:"type" jsonschema:"epic, story, task, bug, or subtask"`
	Status         string       `json:"status" jsonschema:"the issue's status/column name"`
	StatusCategory string       `json:"status_category,omitempty" jsonschema:"todo, inprogress, or done"`
	Priority       string       `json:"priority" jsonschema:"lowest, low, medium, high, or highest"`
	StoryPoints    *int         `json:"story_points,omitempty"`
	Assignee       *AssigneeDTO `json:"assignee,omitempty"`
	EpicKey        string       `json:"epic_key,omitempty"`
	Sprint         string       `json:"sprint,omitempty"`
	Tags           []string     `json:"tags,omitempty"`
	Description    string       `json:"description,omitempty" jsonschema:"Markdown body"`
	CreatedAt      string       `json:"created_at,omitempty"`
	UpdatedAt      string       `json:"updated_at,omitempty"`
}

type LinkDTO struct {
	Type  string `json:"type" jsonschema:"blocks, blocked_by, relates, or duplicates"`
	Key   string `json:"key" jsonschema:"the related issue's key"`
	Title string `json:"title"`
}

type CommentDTO struct {
	Author    string `json:"author" jsonschema:"human or run:<id>"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// IssueDetailDTO is the rich view returned by get_issue: the issue plus its
// links and comments, so an agent starting a ticket has full context in one call.
type IssueDetailDTO struct {
	IssueDTO
	Links    []LinkDTO    `json:"links,omitempty"`
	Comments []CommentDTO `json:"comments,omitempty"`
}

type SprintDTO struct {
	Name  string `json:"name"`
	Goal  string `json:"goal,omitempty"`
	State string `json:"state" jsonschema:"future, active, or completed"`
}

type TagDTO struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// --- mapping helpers ---------------------------------------------------------

func (svc *Service) projectDTO(p core.Project, withStatuses bool) (ProjectDTO, error) {
	dto := ProjectDTO{KeyPrefix: p.KeyPrefix, Name: p.Name, RepoPath: p.RepoPath}
	if withStatuses {
		statuses, err := svc.store.ListStatuses(p.ID)
		if err != nil {
			return ProjectDTO{}, err
		}
		for _, st := range statuses {
			dto.Statuses = append(dto.Statuses, StatusDTO{Name: st.Name, Category: string(st.Category)})
		}
	}
	return dto, nil
}

// issueDTO resolves an issue's status name, epic key, sprint name and tags into
// the agent-facing shape. It performs a few extra reads per issue; at Jeera's
// scale that is fine, and list tools reuse it for consistency.
func (svc *Service) issueDTO(iss core.Issue) (IssueDTO, error) {
	dto := IssueDTO{
		Key:         iss.Key,
		Title:       iss.Title,
		Type:        string(iss.Type),
		Priority:    string(iss.Priority),
		StoryPoints: iss.StoryPoints,
		Description: iss.Description,
		CreatedAt:   formatTime(iss.CreatedAt),
		UpdatedAt:   formatTime(iss.UpdatedAt),
	}
	if st, err := svc.store.GetStatus(iss.StatusID); err == nil {
		dto.Status = st.Name
		dto.StatusCategory = string(st.Category)
	}
	if !iss.Assignee.IsZero() {
		dto.Assignee = &AssigneeDTO{
			Provider: string(iss.Assignee.Provider),
			Model:    iss.Assignee.Model,
			Effort:   string(iss.Assignee.Effort),
		}
	}
	if iss.EpicID != nil {
		if epic, err := svc.store.GetIssue(*iss.EpicID); err == nil {
			dto.EpicKey = epic.Key
		}
	}
	if iss.SprintID != nil {
		if sp, err := svc.store.GetSprint(*iss.SprintID); err == nil {
			dto.Sprint = sp.Name
		}
	}
	tags, err := svc.store.ListIssueTags(iss.ID)
	if err != nil {
		return IssueDTO{}, err
	}
	for _, t := range tags {
		dto.Tags = append(dto.Tags, t.Name)
	}
	return dto, nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// --- resolvers (name/key → row) ----------------------------------------------

func (svc *Service) resolveProject(keyPrefix string) (core.Project, error) {
	p, err := svc.store.GetProjectByPrefix(keyPrefix)
	if err != nil {
		return core.Project{}, fmt.Errorf("project %q not found", keyPrefix)
	}
	return p, nil
}

func (svc *Service) resolveIssue(key string) (core.Issue, error) {
	iss, err := svc.store.GetIssueByKey(key)
	if err != nil {
		return core.Issue{}, fmt.Errorf("issue %q not found", key)
	}
	return iss, nil
}

func (svc *Service) resolveSprint(projectID int64, name string) (core.Sprint, error) {
	sprints, err := svc.store.ListSprints(projectID)
	if err != nil {
		return core.Sprint{}, err
	}
	for _, sp := range sprints {
		if strings.EqualFold(sp.Name, name) {
			return sp, nil
		}
	}
	return core.Sprint{}, fmt.Errorf("sprint %q not found in this project", name)
}
