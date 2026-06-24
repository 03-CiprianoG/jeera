package core

import (
	"errors"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }

// mustTime parses a YYYY-MM-DD date for tests, failing loudly on a bad literal.
func mustTime(date string) time.Time {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		panic(err)
	}
	return t
}

func validIssue() Issue {
	return Issue{
		ProjectID: 1,
		Title:     "Build the kanban board",
		Type:      TypeStory,
		Priority:  PriorityMedium,
		StatusID:  1,
	}
}

func TestIssueValidate(t *testing.T) {
	if err := validIssue().Validate(); err != nil {
		t.Fatalf("valid issue rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Issue)
	}{
		{"no project", func(i *Issue) { i.ProjectID = 0 }},
		{"blank title", func(i *Issue) { i.Title = "   " }},
		{"bad type", func(i *Issue) { i.Type = "spike" }},
		{"bad priority", func(i *Issue) { i.Priority = "urgent" }},
		{"no status", func(i *Issue) { i.StatusID = 0 }},
		{"negative points", func(i *Issue) { i.StoryPoints = ptr(-3) }},
		{"partial assignee", func(i *Issue) { i.Assignee = Assignee{Provider: ProviderClaude} }},
		{"bad assignee provider", func(i *Issue) { i.Assignee = Assignee{Provider: "x", Model: "m"} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iss := validIssue()
			tt.mutate(&iss)
			err := iss.Validate()
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("error %v does not wrap ErrInvalid", err)
			}
		})
	}
}

func TestAssigneeValidate(t *testing.T) {
	if err := (Assignee{}).Validate(); err != nil {
		t.Errorf("zero assignee should be valid, got %v", err)
	}
	if err := (Assignee{Provider: ProviderClaude, Model: "opus", Effort: EffortHigh}).Validate(); err != nil {
		t.Errorf("complete assignee should be valid, got %v", err)
	}
	if err := (Assignee{Provider: ProviderClaude, Model: "opus", Effort: "ludicrous"}).Validate(); err == nil {
		t.Error("bad effort should fail validation")
	}
}

func TestProjectValidate(t *testing.T) {
	good := Project{Name: "Jeera", KeyPrefix: "JEE", RepoPath: "/home/u/jeera"}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid project rejected: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*Project)
	}{
		{"blank name", func(p *Project) { p.Name = "" }},
		{"bad prefix", func(p *Project) { p.KeyPrefix = "1" }},
		{"no repo", func(p *Project) { p.RepoPath = "" }},
		{"bad default provider", func(p *Project) { p.Defaults.Provider = "x" }},
		{"bad default effort", func(p *Project) { p.Defaults.Effort = "x" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := good
			tt.mutate(&p)
			if err := p.Validate(); err == nil || !errors.Is(err, ErrInvalid) {
				t.Errorf("expected ErrInvalid, got %v", err)
			}
		})
	}
}

func TestLinkValidate(t *testing.T) {
	if err := (IssueLink{ProjectID: 1, SourceID: 1, TargetID: 2, Type: LinkBlocks}).Validate(); err != nil {
		t.Errorf("valid link rejected: %v", err)
	}
	bad := []IssueLink{
		{ProjectID: 0, SourceID: 1, TargetID: 2, Type: LinkBlocks},
		{ProjectID: 1, SourceID: 1, TargetID: 1, Type: LinkBlocks},
		{ProjectID: 1, SourceID: 1, TargetID: 0, Type: LinkBlocks},
		{ProjectID: 1, SourceID: 1, TargetID: 2, Type: "clones"},
	}
	for i, l := range bad {
		if err := l.Validate(); err == nil {
			t.Errorf("bad link %d passed validation", i)
		}
	}
}

func TestSprintValidate(t *testing.T) {
	start := mustTime("2026-06-01")
	end := mustTime("2026-06-15")
	if err := (Sprint{ProjectID: 1, Name: "S1", State: SprintActive, StartAt: &start, EndAt: &end}).Validate(); err != nil {
		t.Errorf("valid sprint rejected: %v", err)
	}
	if err := (Sprint{ProjectID: 1, Name: "S1", StartAt: &end, EndAt: &start}).Validate(); err == nil {
		t.Error("sprint with end before start should fail")
	}
	if err := (Sprint{ProjectID: 1, Name: ""}).Validate(); err == nil {
		t.Error("sprint with blank name should fail")
	}
}
