package store

import (
	"errors"
	"testing"
	"time"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// doneStatus and aTodoStatus return a project's done-category and first
// non-done status, so a test can place issues on either side of the
// complete-sprint rollover boundary without hard-coding column order.
func doneStatus(t *testing.T, s *Store, projectID int64) core.Status {
	t.Helper()
	statuses, err := s.ListStatuses(projectID)
	if err != nil {
		t.Fatalf("ListStatuses: %v", err)
	}
	for _, st := range statuses {
		if st.Category == core.CategoryDone {
			return st
		}
	}
	t.Fatalf("no done-category status seeded for project %d", projectID)
	return core.Status{}
}

func TestActiveSprint(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)

	if _, ok, err := s.ActiveSprint(p.ID); err != nil || ok {
		t.Fatalf("a fresh project has no active sprint, got ok=%v err=%v", ok, err)
	}

	// A future and a completed sprint must not count as active.
	if _, err := s.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "later", State: core.SprintFuture}); err != nil {
		t.Fatalf("CreateSprint future: %v", err)
	}
	if _, err := s.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "done", State: core.SprintCompleted}); err != nil {
		t.Fatalf("CreateSprint completed: %v", err)
	}
	if _, ok, _ := s.ActiveSprint(p.ID); ok {
		t.Fatal("future/completed sprints must not be reported as active")
	}

	live, err := s.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "now", State: core.SprintActive})
	if err != nil {
		t.Fatalf("CreateSprint active: %v", err)
	}
	got, ok, err := s.ActiveSprint(p.ID)
	if err != nil || !ok {
		t.Fatalf("expected an active sprint, got ok=%v err=%v", ok, err)
	}
	if got.ID != live.ID {
		t.Errorf("ActiveSprint returned %d, want %d", got.ID, live.ID)
	}
}

func TestStartSprintSingleActive(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)

	s1, _ := s.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "S1"})
	s2, _ := s.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "S2"})

	if err := s.StartSprint(s1.ID); err != nil {
		t.Fatalf("StartSprint(S1): %v", err)
	}
	got, ok, _ := s.ActiveSprint(p.ID)
	if !ok || got.ID != s1.ID {
		t.Fatalf("S1 should be active, got ok=%v id=%d", ok, got.ID)
	}

	// A second active sprint in the same project is rejected as a bad request.
	err := s.StartSprint(s2.ID)
	if !errors.Is(err, core.ErrInvalid) {
		t.Fatalf("starting a second sprint should fail with core.ErrInvalid, got %v", err)
	}
	if got, _, _ := s.ActiveSprint(p.ID); got.ID != s1.ID {
		t.Errorf("S1 must remain the active sprint after the rejected start, got %d", got.ID)
	}

	// Another project's sprint is independent.
	p2, _ := s.CreateProject(core.Project{Name: "Web", KeyPrefix: "WEB", RepoPath: "/w"})
	w1, _ := s.CreateSprint(core.Sprint{ProjectID: p2.ID, Name: "W1"})
	if err := s.StartSprint(w1.ID); err != nil {
		t.Fatalf("StartSprint in a different project should succeed: %v", err)
	}
}

func TestStartSprintPreservesStartAt(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)

	// A sprint planned with an explicit window keeps its StartAt when started.
	planned := time.Date(2020, time.March, 2, 9, 0, 0, 0, time.UTC)
	withDate, _ := s.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "dated", StartAt: ptr(planned)})
	if err := s.StartSprint(withDate.ID); err != nil {
		t.Fatalf("StartSprint(dated): %v", err)
	}
	got, _ := s.GetSprint(withDate.ID)
	if got.StartAt == nil || got.StartAt.Year() != 2020 {
		t.Errorf("StartAt must be preserved, got %v", got.StartAt)
	}

	// A sprint with no window gets one stamped on start.
	p2, _ := s.CreateProject(core.Project{Name: "Web", KeyPrefix: "WEB", RepoPath: "/w"})
	bare, _ := s.CreateSprint(core.Sprint{ProjectID: p2.ID, Name: "bare"})
	if err := s.StartSprint(bare.ID); err != nil {
		t.Fatalf("StartSprint(bare): %v", err)
	}
	if got, _ := s.GetSprint(bare.ID); got.StartAt == nil {
		t.Error("StartAt must be stamped when a sprint starts without one")
	}
}

func TestCompleteSprintRollover(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)
	done := doneStatus(t, s, p.ID)

	sp, _ := s.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "S1", State: core.SprintActive})

	// Two issues left unfinished (they default to the first, todo column), one
	// moved to a done column — all assigned to the sprint.
	a := mustIssue(t, s, p.ID, "unfinished-a")
	b := mustIssue(t, s, p.ID, "unfinished-b")
	shipped := mustIssue(t, s, p.ID, "shipped")
	for _, id := range []int64{a.ID, b.ID, shipped.ID} {
		if err := s.AddIssueToSprint(id, ptr(sp.ID)); err != nil {
			t.Fatalf("AddIssueToSprint(%d): %v", id, err)
		}
	}
	if err := s.TransitionIssue(shipped.ID, done.ID); err != nil {
		t.Fatalf("TransitionIssue(shipped→done): %v", err)
	}

	if err := s.CompleteSprint(sp.ID); err != nil {
		t.Fatalf("CompleteSprint: %v", err)
	}

	closed, _ := s.GetSprint(sp.ID)
	if closed.State != core.SprintCompleted {
		t.Errorf("sprint state = %q, want completed", closed.State)
	}
	if closed.EndAt == nil {
		t.Error("CompleteSprint must stamp EndAt")
	}

	// Unfinished issues fall back to the backlog; the done issue stays attached so
	// the closed sprint records what shipped.
	backlog, _ := s.ListIssues(IssueFilter{ProjectID: p.ID, Unsprinted: true})
	if len(backlog) != 2 {
		t.Errorf("expected 2 issues rolled to backlog, got %d", len(backlog))
	}
	kept, _ := s.ListIssues(IssueFilter{ProjectID: p.ID, SprintID: ptr(sp.ID)})
	if len(kept) != 1 || kept[0].ID != shipped.ID {
		t.Errorf("only the shipped issue should remain on the sprint, got %+v", kept)
	}
}

func TestCompleteSprintNotFound(t *testing.T) {
	s := newTestStore(t)
	if err := s.CompleteSprint(424242); !errors.Is(err, ErrNotFound) {
		t.Errorf("completing a missing sprint should return ErrNotFound, got %v", err)
	}
}

func TestSingleActiveIndexEnforced(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)

	// The partial unique index exists by name.
	var name string
	err := s.DB().QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_sprints_one_active'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("expected idx_sprints_one_active to exist: %v", err)
	}

	a, _ := s.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "A", State: core.SprintActive})
	_ = a
	future, _ := s.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "B", State: core.SprintFuture})

	// Bypass the StartSprint guard with a raw write: the index itself must still
	// reject a second active sprint in the same project.
	if _, err := s.DB().Exec(`UPDATE sprints SET state='active' WHERE id=?`, future.ID); err == nil {
		t.Error("the partial unique index must reject a second active sprint")
	}
}
