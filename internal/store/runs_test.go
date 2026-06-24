package store

import (
	"errors"
	"testing"
	"time"

	"github.com/03-CiprianoG/jeera/internal/core"
)

func TestRunLifecycle(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)
	iss := mustIssue(t, s, p.ID, "runnable")

	r, err := s.CreateRun(core.Run{
		IssueID: iss.ID, Provider: core.ProviderClaude, Model: "opus", Effort: core.EffortHigh,
		Status: core.RunRunning, PermissionMode: "bypassPermissions",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if r.ID == 0 || r.Version != 1 {
		t.Fatalf("expected version 1 with id, got %+v", r)
	}

	if v, _ := s.NextRunVersion(iss.ID); v != 2 {
		t.Errorf("NextRunVersion = %d, want 2", v)
	}

	// Update to a terminal state.
	now := time.Now().UTC()
	code := 0
	r.SessionID = "abc-123"
	r.Status = core.RunSucceeded
	r.EndedAt = &now
	r.ExitCode = &code
	if err := s.UpdateRun(r); err != nil {
		t.Fatalf("UpdateRun: %v", err)
	}

	got, err := s.GetRun(r.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.SessionID != "abc-123" || got.Status != core.RunSucceeded || got.ExitCode == nil || *got.ExitCode != 0 {
		t.Errorf("run not updated: %+v", got)
	}

	runs, _ := s.ListRuns(iss.ID)
	if len(runs) != 1 {
		t.Errorf("ListRuns = %d, want 1", len(runs))
	}

	// A second, active run.
	if _, err := s.CreateRun(core.Run{IssueID: iss.ID, Provider: core.ProviderCodex, Status: core.RunRunning}); err != nil {
		t.Fatalf("CreateRun 2: %v", err)
	}
	if active, _ := s.ListActiveRuns(); len(active) != 1 {
		t.Errorf("ListActiveRuns = %d, want 1", len(active))
	}
	if recent, _ := s.ListRecentRuns(10); len(recent) != 2 {
		t.Errorf("ListRecentRuns = %d, want 2", len(recent))
	}
}

func TestGetRunNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetRun(999); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRunCascadesWithIssue(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)
	iss := mustIssue(t, s, p.ID, "x")
	r, _ := s.CreateRun(core.Run{IssueID: iss.ID, Provider: core.ProviderClaude, Status: core.RunRunning})
	if err := s.DeleteIssue(iss.ID); err != nil {
		t.Fatalf("DeleteIssue: %v", err)
	}
	if _, err := s.GetRun(r.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("run should be cascade-deleted with its issue, got %v", err)
	}
}
