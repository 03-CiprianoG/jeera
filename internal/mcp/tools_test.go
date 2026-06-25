package mcp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/store"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "jeera.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreateProject(core.Project{Name: "Jeera", KeyPrefix: "JEE", RepoPath: "/tmp/jeera"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	return NewService(st)
}

var ctx = context.Background()

func mustCreate(t *testing.T, svc *Service, args CreateIssueArgs) IssueDTO {
	t.Helper()
	_, dto, err := svc.createIssue(ctx, nil, args)
	if err != nil {
		t.Fatalf("create_issue(%+v): %v", args, err)
	}
	return dto
}

func TestCreateIssueDefaultsAndKey(t *testing.T) {
	svc := newTestService(t)
	dto := mustCreate(t, svc, CreateIssueArgs{Project: "JEE", Title: "Build the board"})
	if dto.Key != "JEE-1" {
		t.Errorf("key = %q, want JEE-1", dto.Key)
	}
	if dto.Type != "task" || dto.Priority != "medium" {
		t.Errorf("defaults: type=%q priority=%q", dto.Type, dto.Priority)
	}
	if dto.Status != "To Do" || dto.StatusCategory != "todo" {
		t.Errorf("default status = %q/%q, want To Do/todo", dto.Status, dto.StatusCategory)
	}
}

func TestAddAttachment(t *testing.T) {
	svc := newTestService(t)
	iss := mustCreate(t, svc, CreateIssueArgs{Project: "JEE", Title: "needs a spec"})

	// A URL attachment.
	_, out, err := svc.addAttachment(ctx, nil, AddAttachmentArgs{Key: iss.Key, Ref: "https://example.com/spec"})
	if err != nil {
		t.Fatalf("add_attachment(url): %v", err)
	}
	if !out.Attachment.IsURL || out.Attachment.Path != "https://example.com/spec" {
		t.Errorf("url attachment DTO wrong: %+v", out.Attachment)
	}

	// A file attachment, with a MIME guessed from the extension.
	_, out2, err := svc.addAttachment(ctx, nil, AddAttachmentArgs{Key: iss.Key, Ref: "/docs/diagram.png"})
	if err != nil {
		t.Fatalf("add_attachment(file): %v", err)
	}
	if out2.Attachment.IsURL || out2.Attachment.MIME != "image/png" || out2.Attachment.Filename != "diagram.png" {
		t.Errorf("file attachment DTO wrong: %+v", out2.Attachment)
	}
}

func TestAddAttachmentUnknownIssue(t *testing.T) {
	svc := newTestService(t)
	if _, _, err := svc.addAttachment(ctx, nil, AddAttachmentArgs{Key: "JEE-999", Ref: "https://x.com"}); err == nil {
		t.Fatal("expected an error for an unknown issue")
	}
}

func TestCreateIssueUnknownProject(t *testing.T) {
	svc := newTestService(t)
	if _, _, err := svc.createIssue(ctx, nil, CreateIssueArgs{Project: "NOPE", Title: "x"}); err == nil {
		t.Fatal("expected error for unknown project")
	}
}

func TestGetIssueWithLinksAndComments(t *testing.T) {
	svc := newTestService(t)
	a := mustCreate(t, svc, CreateIssueArgs{Project: "JEE", Title: "A"})
	b := mustCreate(t, svc, CreateIssueArgs{Project: "JEE", Title: "B"})

	if _, _, err := svc.linkIssues(ctx, nil, LinkIssuesArgs{Source: a.Key, Target: b.Key, Type: "blocks"}); err != nil {
		t.Fatalf("link_issues: %v", err)
	}
	if _, _, err := svc.addComment(ctx, nil, AddCommentArgs{Key: a.Key, Body: "first note"}); err != nil {
		t.Fatalf("add_comment: %v", err)
	}

	_, detail, err := svc.getIssue(ctx, nil, GetIssueArgs{Key: a.Key})
	if err != nil {
		t.Fatalf("get_issue: %v", err)
	}
	if len(detail.Links) != 1 || detail.Links[0].Key != b.Key || detail.Links[0].Type != "blocks" {
		t.Errorf("links = %+v, want one blocks->%s", detail.Links, b.Key)
	}
	if len(detail.Comments) != 1 || detail.Comments[0].Body != "first note" || detail.Comments[0].Author != "human" {
		t.Errorf("comments = %+v", detail.Comments)
	}

	// The target shows the inverse relationship.
	_, bDetail, _ := svc.getIssue(ctx, nil, GetIssueArgs{Key: b.Key})
	if len(bDetail.Links) != 1 || bDetail.Links[0].Type != "blocked_by" {
		t.Errorf("target link = %+v, want blocked_by", bDetail.Links)
	}
}

func TestGetIssueNotFound(t *testing.T) {
	svc := newTestService(t)
	if _, _, err := svc.getIssue(ctx, nil, GetIssueArgs{Key: "JEE-99"}); err == nil {
		t.Fatal("expected error for unknown issue")
	}
}

func TestTransitionIssue(t *testing.T) {
	svc := newTestService(t)
	iss := mustCreate(t, svc, CreateIssueArgs{Project: "JEE", Title: "move me"})
	_, dto, err := svc.transitionIssue(ctx, nil, TransitionIssueArgs{Key: iss.Key, Status: "in progress"})
	if err != nil {
		t.Fatalf("transition_issue: %v", err)
	}
	if dto.Status != "In Progress" {
		t.Errorf("status = %q, want In Progress", dto.Status)
	}
	// Unknown status is rejected.
	if _, _, err := svc.transitionIssue(ctx, nil, TransitionIssueArgs{Key: iss.Key, Status: "Nope"}); err == nil {
		t.Error("expected error for unknown status")
	}
}

func TestUpdateIssuePartial(t *testing.T) {
	svc := newTestService(t)
	iss := mustCreate(t, svc, CreateIssueArgs{Project: "JEE", Title: "old", Description: "keep me"})
	newTitle := "new title"
	_, dto, err := svc.updateIssue(ctx, nil, UpdateIssueArgs{Key: iss.Key, Title: &newTitle})
	if err != nil {
		t.Fatalf("update_issue: %v", err)
	}
	if dto.Title != "new title" {
		t.Errorf("title = %q, want new title", dto.Title)
	}
	if dto.Description != "keep me" {
		t.Errorf("description should be unchanged, got %q", dto.Description)
	}
}

func TestSetAssignee(t *testing.T) {
	svc := newTestService(t)
	iss := mustCreate(t, svc, CreateIssueArgs{Project: "JEE", Title: "assign"})
	_, dto, err := svc.setAssignee(ctx, nil, SetAssigneeArgs{Key: iss.Key, Provider: "claude", Model: "opus", Effort: "high"})
	if err != nil {
		t.Fatalf("set_assignee: %v", err)
	}
	if dto.Assignee == nil || dto.Assignee.Provider != "claude" || dto.Assignee.Model != "opus" || dto.Assignee.Effort != "high" {
		t.Errorf("assignee = %+v", dto.Assignee)
	}
}

func TestListIssuesFiltered(t *testing.T) {
	svc := newTestService(t)
	mustCreate(t, svc, CreateIssueArgs{Project: "JEE", Title: "alpha bug", Type: "bug"})
	story := mustCreate(t, svc, CreateIssueArgs{Project: "JEE", Title: "beta story", Type: "story"})
	if _, _, err := svc.transitionIssue(ctx, nil, TransitionIssueArgs{Key: story.Key, Status: "Done"}); err != nil {
		t.Fatalf("transition: %v", err)
	}

	_, all, err := svc.listIssues(ctx, nil, ListIssuesArgs{Project: "JEE"})
	if err != nil || len(all.Issues) != 2 {
		t.Fatalf("list all: %v len=%d", err, len(all.Issues))
	}
	_, bugs, _ := svc.listIssues(ctx, nil, ListIssuesArgs{Project: "JEE", Type: "bug"})
	if len(bugs.Issues) != 1 || bugs.Issues[0].Type != "bug" {
		t.Errorf("type filter = %+v", bugs.Issues)
	}
	_, done, _ := svc.listIssues(ctx, nil, ListIssuesArgs{Project: "JEE", Status: "Done"})
	if len(done.Issues) != 1 || done.Issues[0].Key != story.Key {
		t.Errorf("status filter = %+v", done.Issues)
	}
	_, found, _ := svc.listIssues(ctx, nil, ListIssuesArgs{Project: "JEE", Text: "alpha"})
	if len(found.Issues) != 1 {
		t.Errorf("text filter = %+v", found.Issues)
	}
}

func TestCreateProjectTool(t *testing.T) {
	svc := newTestService(t) // already has JEE
	_, p, err := svc.createProject(ctx, nil, CreateProjectArgs{Name: "Web", KeyPrefix: "web", RepoPath: "/tmp/web"})
	if err != nil {
		t.Fatalf("create_project: %v", err)
	}
	if p.KeyPrefix != "WEB" || p.RepoPath != "/tmp/web" {
		t.Errorf("project = %+v", p)
	}
	if len(p.Statuses) != 4 {
		t.Errorf("new project should be seeded with 4 columns, got %d", len(p.Statuses))
	}
	// Issues can be created in the new project immediately.
	iss := mustCreate(t, svc, CreateIssueArgs{Project: "WEB", Title: "first"})
	if iss.Key != "WEB-1" {
		t.Errorf("key = %q, want WEB-1", iss.Key)
	}
}

func TestProjectsTools(t *testing.T) {
	svc := newTestService(t)
	_, list, err := svc.listProjects(ctx, nil, NoArgs{})
	if err != nil || len(list.Projects) != 1 || list.Projects[0].KeyPrefix != "JEE" {
		t.Fatalf("list_projects = %+v err=%v", list, err)
	}
	_, p, err := svc.getProject(ctx, nil, GetProjectArgs{Project: "JEE"})
	if err != nil {
		t.Fatalf("get_project: %v", err)
	}
	if len(p.Statuses) != 4 || p.Statuses[0].Name != "To Do" {
		t.Errorf("project statuses = %+v", p.Statuses)
	}
}

func TestSprintTools(t *testing.T) {
	svc := newTestService(t)
	// Create a sprint directly via the store (no create_sprint tool yet).
	proj, _ := svc.store.GetProjectByPrefix("JEE")
	if _, err := svc.store.CreateSprint(core.Sprint{ProjectID: proj.ID, Name: "Sprint 1"}); err != nil {
		t.Fatalf("seed sprint: %v", err)
	}
	iss := mustCreate(t, svc, CreateIssueArgs{Project: "JEE", Title: "x"})

	_, list, err := svc.listSprints(ctx, nil, ListSprintsArgs{Project: "JEE"})
	if err != nil || len(list.Sprints) != 1 || list.Sprints[0].Name != "Sprint 1" {
		t.Fatalf("list_sprints = %+v err=%v", list, err)
	}
	_, dto, err := svc.addToSprint(ctx, nil, AddToSprintArgs{Key: iss.Key, Sprint: "Sprint 1"})
	if err != nil || dto.Sprint != "Sprint 1" {
		t.Fatalf("add_to_sprint = %+v err=%v", dto, err)
	}
	// Empty sprint returns to backlog.
	_, back, err := svc.addToSprint(ctx, nil, AddToSprintArgs{Key: iss.Key})
	if err != nil || back.Sprint != "" {
		t.Errorf("backlog return = %+v err=%v", back, err)
	}
}

func TestTagTools(t *testing.T) {
	svc := newTestService(t)
	iss := mustCreate(t, svc, CreateIssueArgs{Project: "JEE", Title: "x"})
	// tag_issue creates the tag on first use.
	_, dto, err := svc.tagIssue(ctx, nil, TagIssueArgs{Key: iss.Key, Tag: "backend"})
	if err != nil {
		t.Fatalf("tag_issue: %v", err)
	}
	if len(dto.Tags) != 1 || dto.Tags[0] != "backend" {
		t.Errorf("tags = %v", dto.Tags)
	}
	_, list, _ := svc.listTags(ctx, nil, ListTagsArgs{Project: "JEE"})
	if len(list.Tags) != 1 || list.Tags[0].Name != "backend" {
		t.Errorf("list_tags = %+v", list.Tags)
	}
	// Re-tagging with the same name reuses the tag (no duplicate).
	if _, _, err := svc.tagIssue(ctx, nil, TagIssueArgs{Key: iss.Key, Tag: "BACKEND"}); err != nil {
		t.Fatalf("re-tag: %v", err)
	}
	_, list2, _ := svc.listTags(ctx, nil, ListTagsArgs{Project: "JEE"})
	if len(list2.Tags) != 1 {
		t.Errorf("expected 1 tag after case-insensitive re-tag, got %d", len(list2.Tags))
	}
}

func TestClientConfigJSON(t *testing.T) {
	svc := newTestService(t)
	srv := NewServer(svc)
	cfg := srv.ClientConfigJSON()
	for _, want := range []string{`"mcpServers"`, `"jeera"`, `"type": "http"`, `"url"`} {
		if !strings.Contains(cfg, want) {
			t.Errorf("config JSON missing %q:\n%s", want, cfg)
		}
	}
}
