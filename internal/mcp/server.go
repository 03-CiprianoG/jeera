package mcp

import (
	"github.com/03-CiprianoG/jeera/internal/version"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewMCPServer builds an MCP server exposing every Jeera tool over the store.
func NewMCPServer(svc *Service) *mcpsdk.Server {
	s := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "jeera",
		Title:   "Jeera",
		Version: version.Short(),
	}, nil)
	RegisterAll(s, svc)
	return s
}

// RegisterAll registers Jeera's tools on an MCP server. Each tool's input and
// output schemas are inferred from its typed argument and result structs.
func RegisterAll(s *mcpsdk.Server, svc *Service) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "create_project",
		Description: "Create a Jeera project (board) with a key prefix, bound to a git repository."}, svc.createProject)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "list_projects",
		Description: "List all Jeera projects."}, svc.listProjects)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "get_project",
		Description: "Get a project and its board columns by key prefix."}, svc.getProject)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "list_issues",
		Description: "List issues in a project, optionally filtered by status, sprint, epic, type or a text search."}, svc.listIssues)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "get_issue",
		Description: "Get a single issue by key, including its relationships and comments."}, svc.getIssue)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "create_issue",
		Description: "Create an issue in a project."}, svc.createIssue)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "update_issue",
		Description: "Update fields of an existing issue; only the fields provided are changed."}, svc.updateIssue)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "transition_issue",
		Description: "Move an issue to a different status/column, by status name."}, svc.transitionIssue)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "set_assignee",
		Description: "Assign an issue to a model: a provider, model and reasoning effort."}, svc.setAssignee)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "add_comment",
		Description: "Add a comment to an issue's activity timeline."}, svc.addComment)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "link_issues",
		Description: "Create a relationship (blocks, blocked_by, relates, duplicates) between two issues."}, svc.linkIssues)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "list_sprints",
		Description: "List a project's sprints."}, svc.listSprints)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "add_to_sprint",
		Description: "Add an issue to a sprint, or return it to the backlog when no sprint is given."}, svc.addToSprint)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "list_tags",
		Description: "List a project's tags."}, svc.listTags)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "tag_issue",
		Description: "Add a tag to an issue, creating the tag in the project if it does not exist."}, svc.tagIssue)
}
