package mcp

import (
	"context"
	"fmt"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/store"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- get_issue ---------------------------------------------------------------

type GetIssueArgs struct {
	Key string `json:"key" jsonschema:"the issue key, e.g. JEE-12"`
}

func (svc *Service) getIssue(_ context.Context, _ *mcpsdk.CallToolRequest, args GetIssueArgs) (*mcpsdk.CallToolResult, IssueDetailDTO, error) {
	iss, err := svc.resolveIssue(args.Key)
	if err != nil {
		return nil, IssueDetailDTO{}, err
	}
	base, err := svc.issueDTO(iss)
	if err != nil {
		return nil, IssueDetailDTO{}, err
	}
	detail := IssueDetailDTO{IssueDTO: base}

	links, err := svc.store.ListLinks(iss.ID)
	if err != nil {
		return nil, IssueDetailDTO{}, err
	}
	for _, l := range links {
		detail.Links = append(detail.Links, LinkDTO{Type: string(l.Type), Key: l.Issue.Key, Title: l.Issue.Title})
	}
	comments, err := svc.store.ListComments(iss.ID)
	if err != nil {
		return nil, IssueDetailDTO{}, err
	}
	for _, c := range comments {
		detail.Comments = append(detail.Comments, CommentDTO{Author: c.Author, Body: c.Body, CreatedAt: formatTime(c.CreatedAt)})
	}
	return nil, detail, nil
}

// --- list_issues -------------------------------------------------------------

type ListIssuesArgs struct {
	Project string `json:"project" jsonschema:"the project key prefix, e.g. JEE"`
	Status  string `json:"status,omitempty" jsonschema:"filter by status/column name"`
	Sprint  string `json:"sprint,omitempty" jsonschema:"filter by sprint name"`
	Epic    string `json:"epic,omitempty" jsonschema:"filter by parent epic key"`
	Type    string `json:"type,omitempty" jsonschema:"filter by issue type"`
	Text    string `json:"text,omitempty" jsonschema:"search the title and description"`
}

type IssueListResult struct {
	Issues []IssueDTO `json:"issues"`
}

func (svc *Service) listIssues(_ context.Context, _ *mcpsdk.CallToolRequest, args ListIssuesArgs) (*mcpsdk.CallToolResult, IssueListResult, error) {
	p, err := svc.resolveProject(args.Project)
	if err != nil {
		return nil, IssueListResult{}, err
	}
	f := store.IssueFilter{ProjectID: p.ID, Text: args.Text}
	if args.Type != "" {
		f.Type = core.IssueType(args.Type)
	}
	if args.Status != "" {
		st, err := svc.store.StatusByName(p.ID, args.Status)
		if err != nil {
			return nil, IssueListResult{}, fmt.Errorf("status %q not found in project %s", args.Status, p.KeyPrefix)
		}
		f.StatusID = st.ID
	}
	if args.Sprint != "" {
		sp, err := svc.resolveSprint(p.ID, args.Sprint)
		if err != nil {
			return nil, IssueListResult{}, err
		}
		f.SprintID = &sp.ID
	}
	if args.Epic != "" {
		epic, err := svc.resolveIssue(args.Epic)
		if err != nil {
			return nil, IssueListResult{}, err
		}
		f.EpicID = &epic.ID
	}

	issues, err := svc.store.ListIssues(f)
	if err != nil {
		return nil, IssueListResult{}, err
	}
	out := IssueListResult{Issues: make([]IssueDTO, 0, len(issues))}
	for _, iss := range issues {
		dto, err := svc.issueDTO(iss)
		if err != nil {
			return nil, IssueListResult{}, err
		}
		out.Issues = append(out.Issues, dto)
	}
	return nil, out, nil
}

// --- create_issue ------------------------------------------------------------

type CreateIssueArgs struct {
	Project     string `json:"project" jsonschema:"the project key prefix, e.g. JEE"`
	Title       string `json:"title"`
	Type        string `json:"type,omitempty" jsonschema:"epic, story, task, bug, or subtask (default task)"`
	Description string `json:"description,omitempty" jsonschema:"Markdown body"`
	Priority    string `json:"priority,omitempty" jsonschema:"lowest, low, medium, high, highest (default medium)"`
	StoryPoints *int   `json:"story_points,omitempty"`
	Status      string `json:"status,omitempty" jsonschema:"initial status name (default first column)"`
	Epic        string `json:"epic,omitempty" jsonschema:"parent epic key"`
	Sprint      string `json:"sprint,omitempty" jsonschema:"sprint name"`
}

func (svc *Service) createIssue(_ context.Context, _ *mcpsdk.CallToolRequest, args CreateIssueArgs) (*mcpsdk.CallToolResult, IssueDTO, error) {
	p, err := svc.resolveProject(args.Project)
	if err != nil {
		return nil, IssueDTO{}, err
	}
	iss := core.Issue{
		ProjectID:   p.ID,
		Title:       args.Title,
		Type:        core.IssueType(args.Type),
		Description: args.Description,
		Priority:    core.Priority(args.Priority),
		StoryPoints: args.StoryPoints,
	}
	if args.Status != "" {
		st, err := svc.store.StatusByName(p.ID, args.Status)
		if err != nil {
			return nil, IssueDTO{}, fmt.Errorf("status %q not found in project %s", args.Status, p.KeyPrefix)
		}
		iss.StatusID = st.ID
	}
	if args.Epic != "" {
		epic, err := svc.resolveIssue(args.Epic)
		if err != nil {
			return nil, IssueDTO{}, err
		}
		if epic.Type != core.TypeEpic {
			return nil, IssueDTO{}, fmt.Errorf("issue %q is not an epic", args.Epic)
		}
		iss.EpicID = &epic.ID
	}
	if args.Sprint != "" {
		sp, err := svc.resolveSprint(p.ID, args.Sprint)
		if err != nil {
			return nil, IssueDTO{}, err
		}
		iss.SprintID = &sp.ID
	}

	created, err := svc.store.CreateIssue(iss)
	if err != nil {
		return nil, IssueDTO{}, err
	}
	dto, err := svc.issueDTO(created)
	return nil, dto, err
}

// --- update_issue (partial) --------------------------------------------------

type UpdateIssueArgs struct {
	Key         string  `json:"key" jsonschema:"the issue key, e.g. JEE-12"`
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty" jsonschema:"Markdown body"`
	Priority    *string `json:"priority,omitempty"`
	Type        *string `json:"type,omitempty"`
	StoryPoints *int    `json:"story_points,omitempty"`
}

func (svc *Service) updateIssue(_ context.Context, _ *mcpsdk.CallToolRequest, args UpdateIssueArgs) (*mcpsdk.CallToolResult, IssueDTO, error) {
	iss, err := svc.resolveIssue(args.Key)
	if err != nil {
		return nil, IssueDTO{}, err
	}
	if args.Title != nil {
		iss.Title = *args.Title
	}
	if args.Description != nil {
		iss.Description = *args.Description
	}
	if args.Priority != nil {
		iss.Priority = core.Priority(*args.Priority)
	}
	if args.Type != nil {
		iss.Type = core.IssueType(*args.Type)
	}
	if args.StoryPoints != nil {
		iss.StoryPoints = args.StoryPoints
	}
	if err := svc.store.UpdateIssue(iss); err != nil {
		return nil, IssueDTO{}, err
	}
	updated, err := svc.resolveIssue(args.Key)
	if err != nil {
		return nil, IssueDTO{}, err
	}
	dto, err := svc.issueDTO(updated)
	return nil, dto, err
}

// --- transition_issue --------------------------------------------------------

type TransitionIssueArgs struct {
	Key    string `json:"key" jsonschema:"the issue key, e.g. JEE-12"`
	Status string `json:"status" jsonschema:"the target status/column name, e.g. In Progress"`
}

func (svc *Service) transitionIssue(_ context.Context, _ *mcpsdk.CallToolRequest, args TransitionIssueArgs) (*mcpsdk.CallToolResult, IssueDTO, error) {
	iss, err := svc.resolveIssue(args.Key)
	if err != nil {
		return nil, IssueDTO{}, err
	}
	st, err := svc.store.StatusByName(iss.ProjectID, args.Status)
	if err != nil {
		return nil, IssueDTO{}, fmt.Errorf("status %q not found for issue %s", args.Status, iss.Key)
	}
	if err := svc.store.TransitionIssue(iss.ID, st.ID); err != nil {
		return nil, IssueDTO{}, err
	}
	updated, err := svc.store.GetIssue(iss.ID)
	if err != nil {
		return nil, IssueDTO{}, err
	}
	dto, err := svc.issueDTO(updated)
	return nil, dto, err
}

// --- set_assignee ------------------------------------------------------------

type SetAssigneeArgs struct {
	Key      string `json:"key" jsonschema:"the issue key, e.g. JEE-12"`
	Provider string `json:"provider" jsonschema:"claude or codex"`
	Model    string `json:"model" jsonschema:"the model name/alias, e.g. opus"`
	Effort   string `json:"effort,omitempty" jsonschema:"reasoning effort, e.g. high"`
}

func (svc *Service) setAssignee(_ context.Context, _ *mcpsdk.CallToolRequest, args SetAssigneeArgs) (*mcpsdk.CallToolResult, IssueDTO, error) {
	iss, err := svc.resolveIssue(args.Key)
	if err != nil {
		return nil, IssueDTO{}, err
	}
	iss.Assignee = core.Assignee{
		Provider: core.Provider(args.Provider),
		Model:    args.Model,
		Effort:   core.Effort(args.Effort),
	}
	if err := svc.store.UpdateIssue(iss); err != nil {
		return nil, IssueDTO{}, err
	}
	updated, err := svc.store.GetIssue(iss.ID)
	if err != nil {
		return nil, IssueDTO{}, err
	}
	dto, err := svc.issueDTO(updated)
	return nil, dto, err
}
