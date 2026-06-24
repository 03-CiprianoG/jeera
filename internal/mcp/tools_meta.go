package mcp

import (
	"context"
	"os"
	"strings"

	"github.com/03-CiprianoG/jeera/internal/core"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// NoArgs is the input type for tools that take no parameters; its empty-struct
// schema is a valid JSON "object", which the SDK requires.
type NoArgs struct{}

// --- create_project ----------------------------------------------------------

type CreateProjectArgs struct {
	Name      string `json:"name"`
	KeyPrefix string `json:"key_prefix" jsonschema:"2-10 letters/digits starting with a letter, e.g. JEE"`
	RepoPath  string `json:"repo_path,omitempty" jsonschema:"absolute path to the project's git repo; defaults to the server's working directory"`
}

func (svc *Service) createProject(_ context.Context, _ *mcpsdk.CallToolRequest, args CreateProjectArgs) (*mcpsdk.CallToolResult, ProjectDTO, error) {
	repo := strings.TrimSpace(args.RepoPath)
	if repo == "" {
		if wd, err := os.Getwd(); err == nil {
			repo = wd
		}
	}
	p, err := svc.store.CreateProject(core.Project{Name: args.Name, KeyPrefix: args.KeyPrefix, RepoPath: repo})
	if err != nil {
		return nil, ProjectDTO{}, err
	}
	dto, err := svc.projectDTO(p, true)
	return nil, dto, err
}

// --- list_projects -----------------------------------------------------------

type ProjectListResult struct {
	Projects []ProjectDTO `json:"projects"`
}

func (svc *Service) listProjects(_ context.Context, _ *mcpsdk.CallToolRequest, _ NoArgs) (*mcpsdk.CallToolResult, ProjectListResult, error) {
	projects, err := svc.store.ListProjects()
	if err != nil {
		return nil, ProjectListResult{}, err
	}
	out := ProjectListResult{Projects: make([]ProjectDTO, 0, len(projects))}
	for _, p := range projects {
		dto, err := svc.projectDTO(p, false)
		if err != nil {
			return nil, ProjectListResult{}, err
		}
		out.Projects = append(out.Projects, dto)
	}
	return nil, out, nil
}

// --- get_project -------------------------------------------------------------

type GetProjectArgs struct {
	Project string `json:"project" jsonschema:"the project key prefix, e.g. JEE"`
}

func (svc *Service) getProject(_ context.Context, _ *mcpsdk.CallToolRequest, args GetProjectArgs) (*mcpsdk.CallToolResult, ProjectDTO, error) {
	p, err := svc.resolveProject(args.Project)
	if err != nil {
		return nil, ProjectDTO{}, err
	}
	dto, err := svc.projectDTO(p, true)
	return nil, dto, err
}

// --- list_sprints ------------------------------------------------------------

type ListSprintsArgs struct {
	Project string `json:"project" jsonschema:"the project key prefix, e.g. JEE"`
}

type SprintListResult struct {
	Sprints []SprintDTO `json:"sprints"`
}

func (svc *Service) listSprints(_ context.Context, _ *mcpsdk.CallToolRequest, args ListSprintsArgs) (*mcpsdk.CallToolResult, SprintListResult, error) {
	p, err := svc.resolveProject(args.Project)
	if err != nil {
		return nil, SprintListResult{}, err
	}
	sprints, err := svc.store.ListSprints(p.ID)
	if err != nil {
		return nil, SprintListResult{}, err
	}
	out := SprintListResult{Sprints: make([]SprintDTO, 0, len(sprints))}
	for _, sp := range sprints {
		out.Sprints = append(out.Sprints, SprintDTO{Name: sp.Name, Goal: sp.Goal, State: string(sp.State)})
	}
	return nil, out, nil
}

// --- add_to_sprint -----------------------------------------------------------

type AddToSprintArgs struct {
	Key    string `json:"key" jsonschema:"the issue key, e.g. JEE-12"`
	Sprint string `json:"sprint,omitempty" jsonschema:"the sprint name; empty returns the issue to the backlog"`
}

func (svc *Service) addToSprint(_ context.Context, _ *mcpsdk.CallToolRequest, args AddToSprintArgs) (*mcpsdk.CallToolResult, IssueDTO, error) {
	iss, err := svc.resolveIssue(args.Key)
	if err != nil {
		return nil, IssueDTO{}, err
	}
	var sprintID *int64
	if strings.TrimSpace(args.Sprint) != "" {
		sp, err := svc.resolveSprint(iss.ProjectID, args.Sprint)
		if err != nil {
			return nil, IssueDTO{}, err
		}
		sprintID = &sp.ID
	}
	if err := svc.store.AddIssueToSprint(iss.ID, sprintID); err != nil {
		return nil, IssueDTO{}, err
	}
	updated, err := svc.store.GetIssue(iss.ID)
	if err != nil {
		return nil, IssueDTO{}, err
	}
	dto, err := svc.issueDTO(updated)
	return nil, dto, err
}

// --- list_tags ---------------------------------------------------------------

type ListTagsArgs struct {
	Project string `json:"project" jsonschema:"the project key prefix, e.g. JEE"`
}

type TagListResult struct {
	Tags []TagDTO `json:"tags"`
}

func (svc *Service) listTags(_ context.Context, _ *mcpsdk.CallToolRequest, args ListTagsArgs) (*mcpsdk.CallToolResult, TagListResult, error) {
	p, err := svc.resolveProject(args.Project)
	if err != nil {
		return nil, TagListResult{}, err
	}
	tags, err := svc.store.ListTags(p.ID)
	if err != nil {
		return nil, TagListResult{}, err
	}
	out := TagListResult{Tags: make([]TagDTO, 0, len(tags))}
	for _, t := range tags {
		out.Tags = append(out.Tags, TagDTO{Name: t.Name, Color: t.Color})
	}
	return nil, out, nil
}

// --- tag_issue ---------------------------------------------------------------

type TagIssueArgs struct {
	Key string `json:"key" jsonschema:"the issue key, e.g. JEE-12"`
	Tag string `json:"tag" jsonschema:"the tag name; it is created in the project if it does not exist"`
}

func (svc *Service) tagIssue(_ context.Context, _ *mcpsdk.CallToolRequest, args TagIssueArgs) (*mcpsdk.CallToolResult, IssueDTO, error) {
	iss, err := svc.resolveIssue(args.Key)
	if err != nil {
		return nil, IssueDTO{}, err
	}
	tag, err := svc.findOrCreateTag(iss.ProjectID, args.Tag)
	if err != nil {
		return nil, IssueDTO{}, err
	}
	if err := svc.store.TagIssue(iss.ID, tag.ID); err != nil {
		return nil, IssueDTO{}, err
	}
	updated, err := svc.store.GetIssue(iss.ID)
	if err != nil {
		return nil, IssueDTO{}, err
	}
	dto, err := svc.issueDTO(updated)
	return nil, dto, err
}

func (svc *Service) findOrCreateTag(projectID int64, name string) (core.Tag, error) {
	tags, err := svc.store.ListTags(projectID)
	if err != nil {
		return core.Tag{}, err
	}
	for _, t := range tags {
		if strings.EqualFold(t.Name, name) {
			return t, nil
		}
	}
	return svc.store.CreateTag(core.Tag{ProjectID: projectID, Name: name})
}
