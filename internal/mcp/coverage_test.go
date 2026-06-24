package mcp

import (
	"testing"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// TestUpdateIssuePreservesUnsetFields is the most important MCP regression: a
// partial update must change only the fields provided and leave assignee, epic,
// sprint and tags intact.
func TestUpdateIssuePreservesUnsetFields(t *testing.T) {
	svc := newTestService(t)
	proj, _ := svc.store.GetProjectByPrefix("JEE")
	epic := mustCreate(t, svc, CreateIssueArgs{Project: "JEE", Title: "the epic", Type: "epic"})
	if _, err := svc.store.CreateSprint(core.Sprint{ProjectID: proj.ID, Name: "S1"}); err != nil {
		t.Fatalf("seed sprint: %v", err)
	}
	iss := mustCreate(t, svc, CreateIssueArgs{Project: "JEE", Title: "child", Epic: epic.Key})

	if _, _, err := svc.setAssignee(ctx, nil, SetAssigneeArgs{Key: iss.Key, Provider: "claude", Model: "opus", Effort: "high"}); err != nil {
		t.Fatalf("set_assignee: %v", err)
	}
	if _, _, err := svc.addToSprint(ctx, nil, AddToSprintArgs{Key: iss.Key, Sprint: "S1"}); err != nil {
		t.Fatalf("add_to_sprint: %v", err)
	}
	if _, _, err := svc.tagIssue(ctx, nil, TagIssueArgs{Key: iss.Key, Tag: "backend"}); err != nil {
		t.Fatalf("tag_issue: %v", err)
	}

	newTitle := "renamed child"
	_, dto, err := svc.updateIssue(ctx, nil, UpdateIssueArgs{Key: iss.Key, Title: &newTitle})
	if err != nil {
		t.Fatalf("update_issue: %v", err)
	}
	if dto.Title != "renamed child" {
		t.Errorf("title not updated: %q", dto.Title)
	}
	if dto.Assignee == nil || dto.Assignee.Model != "opus" {
		t.Errorf("assignee not preserved: %+v", dto.Assignee)
	}
	if dto.EpicKey != epic.Key {
		t.Errorf("epic not preserved: %q", dto.EpicKey)
	}
	if dto.Sprint != "S1" {
		t.Errorf("sprint not preserved: %q", dto.Sprint)
	}
	if len(dto.Tags) != 1 || dto.Tags[0] != "backend" {
		t.Errorf("tags not preserved: %v", dto.Tags)
	}
}

func TestCreateIssueEpicMustBeEpic(t *testing.T) {
	svc := newTestService(t)
	story := mustCreate(t, svc, CreateIssueArgs{Project: "JEE", Title: "not an epic", Type: "story"})
	if _, _, err := svc.createIssue(ctx, nil, CreateIssueArgs{Project: "JEE", Title: "child", Epic: story.Key}); err == nil {
		t.Fatal("expected error when the epic parent is not an epic")
	}
}

func TestCreateProjectValidation(t *testing.T) {
	svc := newTestService(t) // JEE already exists
	if _, _, err := svc.createProject(ctx, nil, CreateProjectArgs{Name: "Dup", KeyPrefix: "JEE", RepoPath: "/x"}); err == nil {
		t.Error("duplicate prefix should error")
	}
	if _, _, err := svc.createProject(ctx, nil, CreateProjectArgs{Name: "Bad", KeyPrefix: "1", RepoPath: "/x"}); err == nil {
		t.Error("invalid prefix should error")
	}
	if _, _, err := svc.createProject(ctx, nil, CreateProjectArgs{Name: "", KeyPrefix: "OK", RepoPath: "/x"}); err == nil {
		t.Error("blank name should error")
	}
}

func TestSetAssigneeOverwrite(t *testing.T) {
	svc := newTestService(t)
	iss := mustCreate(t, svc, CreateIssueArgs{Project: "JEE", Title: "x"})
	if _, _, err := svc.setAssignee(ctx, nil, SetAssigneeArgs{Key: iss.Key, Provider: "claude", Model: "opus", Effort: "high"}); err != nil {
		t.Fatalf("first assign: %v", err)
	}
	// A second call overwrites wholesale, including clearing the omitted effort.
	_, dto, err := svc.setAssignee(ctx, nil, SetAssigneeArgs{Key: iss.Key, Provider: "codex", Model: "gpt-5.4"})
	if err != nil {
		t.Fatalf("overwrite assign: %v", err)
	}
	if dto.Assignee == nil || dto.Assignee.Provider != "codex" || dto.Assignee.Model != "gpt-5.4" || dto.Assignee.Effort != "" {
		t.Errorf("assignee overwrite = %+v", dto.Assignee)
	}
}

func TestFindOrCreateTagReusesCaseVariant(t *testing.T) {
	svc := newTestService(t)
	iss := mustCreate(t, svc, CreateIssueArgs{Project: "JEE", Title: "x"})
	if _, _, err := svc.tagIssue(ctx, nil, TagIssueArgs{Key: iss.Key, Tag: "Backend"}); err != nil {
		t.Fatalf("tag: %v", err)
	}
	// A different-case tag name reuses the existing tag rather than creating a
	// second one or erroring.
	if _, _, err := svc.tagIssue(ctx, nil, TagIssueArgs{Key: iss.Key, Tag: "backend"}); err != nil {
		t.Fatalf("re-tag case variant: %v", err)
	}
	_, list, _ := svc.listTags(ctx, nil, ListTagsArgs{Project: "JEE"})
	if len(list.Tags) != 1 {
		t.Errorf("expected 1 tag after case-variant reuse, got %d", len(list.Tags))
	}
}

func TestUpdateIssueUnknownKey(t *testing.T) {
	svc := newTestService(t)
	bad := "does not matter"
	if _, _, err := svc.updateIssue(ctx, nil, UpdateIssueArgs{Key: "JEE-999", Title: &bad}); err == nil {
		t.Fatal("expected error updating unknown issue")
	}
}
