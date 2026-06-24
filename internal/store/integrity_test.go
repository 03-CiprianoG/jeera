package store

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/03-CiprianoG/jeera/internal/core"
)

func TestForeignKeysEnforced(t *testing.T) {
	s := newTestStore(t)
	var fk int
	if err := s.DB().QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys = %d, want 1 (cascades/SET NULL depend on it)", fk)
	}
	// A raw insert referencing non-existent parents must be rejected.
	_, err := s.DB().Exec(
		`INSERT INTO issues (project_id, seq, type, title, status_id, priority, rank, settings, created_at, updated_at)
		 VALUES (99999, 1, 'task', 'x', 99999, 'medium', 'r', '{}', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	if err == nil {
		t.Fatal("expected FOREIGN KEY constraint failure for bogus project/status")
	}
}

func TestCrossProjectRefsRejected(t *testing.T) {
	s := newTestStore(t)
	a := mustProject(t, s)
	b, _ := s.CreateProject(core.Project{Name: "B", KeyPrefix: "BEE", RepoPath: "/b"})
	bStatuses, _ := s.ListStatuses(b.ID)
	bSprint, _ := s.CreateSprint(core.Sprint{ProjectID: b.ID, Name: "B-sprint"})
	bIssue := mustIssue(t, s, b.ID, "b issue")

	// CreateIssue in A using B's status/epic/parent/sprint must each be rejected.
	cases := []struct {
		name   string
		mutate func(*core.Issue)
	}{
		{"foreign status", func(i *core.Issue) { i.StatusID = bStatuses[0].ID }},
		{"foreign epic", func(i *core.Issue) { i.EpicID = &bIssue.ID }},
		{"foreign parent", func(i *core.Issue) { i.ParentID = &bIssue.ID }},
		{"foreign sprint", func(i *core.Issue) { i.SprintID = &bSprint.ID }},
	}
	for _, c := range cases {
		t.Run("create/"+c.name, func(t *testing.T) {
			iss := core.Issue{ProjectID: a.ID, Title: "x", Type: core.TypeStory}
			c.mutate(&iss)
			if _, err := s.CreateIssue(iss); err == nil || !errors.Is(err, core.ErrInvalid) {
				t.Fatalf("expected ErrInvalid, got %v", err)
			}
		})
	}

	// UpdateIssue path: take a valid issue in A and try to point it at B's sprint.
	aIssue := mustIssue(t, s, a.ID, "a issue")
	aIssue.SprintID = &bSprint.ID
	if err := s.UpdateIssue(aIssue); err == nil || !errors.Is(err, core.ErrInvalid) {
		t.Errorf("UpdateIssue with foreign sprint: expected ErrInvalid, got %v", err)
	}

	// AddIssueToSprint must reject a sprint from another project too.
	if err := s.AddIssueToSprint(aIssue.ID, &bSprint.ID); err == nil || !errors.Is(err, core.ErrInvalid) {
		t.Errorf("AddIssueToSprint cross-project: expected ErrInvalid, got %v", err)
	}
}

func TestCreateIssueRejectedWhenProjectHasNoStatuses(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)
	// Remove the seeded columns, then a defaulted-status create must fail clearly.
	if _, err := s.DB().Exec(`DELETE FROM statuses WHERE project_id = ?`, p.ID); err != nil {
		t.Fatalf("clear statuses: %v", err)
	}
	if _, err := s.CreateIssue(core.Issue{ProjectID: p.ID, Title: "x"}); err == nil || !errors.Is(err, core.ErrInvalid) {
		t.Fatalf("expected ErrInvalid for project with no statuses, got %v", err)
	}
}

func TestCreateIssueSeqConcurrent(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)

	const n = 50
	var wg sync.WaitGroup
	seqs := make([]int64, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			iss, err := s.CreateIssue(core.Issue{ProjectID: p.ID, Title: "concurrent"})
			seqs[i] = iss.Seq
			errs[i] = err
		}(i)
	}
	wg.Wait()

	seen := make(map[int64]bool, n)
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("CreateIssue %d: %v", i, errs[i])
		}
		if seen[seqs[i]] {
			t.Fatalf("duplicate seq %d allocated", seqs[i])
		}
		seen[seqs[i]] = true
	}
	for want := int64(1); want <= n; want++ {
		if !seen[want] {
			t.Errorf("missing seq %d in concurrent allocation", want)
		}
	}
}

func TestDeleteSprintAndEpicSetNull(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)

	// Deleting a sprint returns its issue to the backlog (SET NULL), not delete it.
	sprint, _ := s.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "S"})
	iss := mustIssue(t, s, p.ID, "in sprint")
	if err := s.AddIssueToSprint(iss.ID, &sprint.ID); err != nil {
		t.Fatalf("AddIssueToSprint: %v", err)
	}
	if err := s.DeleteSprint(sprint.ID); err != nil {
		t.Fatalf("DeleteSprint: %v", err)
	}
	got, err := s.GetIssue(iss.ID)
	if err != nil {
		t.Fatalf("issue should survive sprint deletion: %v", err)
	}
	if got.SprintID != nil {
		t.Errorf("expected SprintID nil after sprint delete, got %v", *got.SprintID)
	}

	// Deleting an epic nulls its children's epic_id rather than deleting them.
	epic, err := s.CreateIssue(core.Issue{ProjectID: p.ID, Title: "epic", Type: core.TypeEpic})
	if err != nil {
		t.Fatalf("create epic: %v", err)
	}
	child, err := s.CreateIssue(core.Issue{ProjectID: p.ID, Title: "child", Type: core.TypeStory, EpicID: &epic.ID})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if err := s.DeleteIssue(epic.ID); err != nil {
		t.Fatalf("DeleteIssue(epic): %v", err)
	}
	gotChild, err := s.GetIssue(child.ID)
	if err != nil {
		t.Fatalf("child should survive epic deletion: %v", err)
	}
	if gotChild.EpicID != nil {
		t.Errorf("expected child EpicID nil after epic delete, got %v", *gotChild.EpicID)
	}
}

func TestNullableInheritVsExplicit(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)

	iss, err := s.CreateIssue(core.Issue{ProjectID: p.ID, Title: "nullable"})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if iss.WorktreeOn != nil || iss.StoryPoints != nil {
		t.Fatalf("expected nil worktree/points on create, got %v / %v", iss.WorktreeOn, iss.StoryPoints)
	}

	// Explicit "off"/zero must be distinguishable from "inherit"/unset.
	iss.WorktreeOn = ptr(false)
	iss.StoryPoints = ptr(0)
	if err := s.UpdateIssue(iss); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	got, _ := s.GetIssue(iss.ID)
	if got.WorktreeOn == nil || *got.WorktreeOn != false {
		t.Errorf("explicit worktree=false lost: %v", got.WorktreeOn)
	}
	if got.StoryPoints == nil || *got.StoryPoints != 0 {
		t.Errorf("explicit points=0 lost: %v", got.StoryPoints)
	}

	got.WorktreeOn = ptr(true)
	if err := s.UpdateIssue(got); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	again, _ := s.GetIssue(iss.ID)
	if again.WorktreeOn == nil || *again.WorktreeOn != true {
		t.Errorf("worktree=true round-trip failed: %v", again.WorktreeOn)
	}
}

func TestCreateTagUniquePerProject(t *testing.T) {
	s := newTestStore(t)
	a := mustProject(t, s)
	b, _ := s.CreateProject(core.Project{Name: "B", KeyPrefix: "BEE", RepoPath: "/b"})

	if _, err := s.CreateTag(core.Tag{ProjectID: a.ID, Name: "bug"}); err != nil {
		t.Fatalf("first tag: %v", err)
	}
	if _, err := s.CreateTag(core.Tag{ProjectID: a.ID, Name: "bug"}); err == nil {
		t.Error("duplicate tag name within a project should be rejected")
	}
	// Same name in a different project is fine.
	if _, err := s.CreateTag(core.Tag{ProjectID: b.ID, Name: "bug"}); err != nil {
		t.Errorf("same tag name in another project should be allowed: %v", err)
	}
	// Validation.
	if _, err := s.CreateTag(core.Tag{ProjectID: 0, Name: "x"}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("expected ErrInvalid for missing project, got %v", err)
	}
	if _, err := s.CreateTag(core.Tag{ProjectID: a.ID, Name: "  "}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("expected ErrInvalid for blank name, got %v", err)
	}
}

func TestAddCommentValidation(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)
	iss := mustIssue(t, s, p.ID, "x")

	if _, err := s.AddComment(core.Comment{IssueID: iss.ID, Body: "  "}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("blank body should be ErrInvalid, got %v", err)
	}
	if _, err := s.AddComment(core.Comment{IssueID: 0, Body: "hi"}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("missing issue id should be ErrInvalid, got %v", err)
	}
	if _, err := s.AddComment(core.Comment{IssueID: 999999, Body: "hi"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("comment on missing issue should be ErrNotFound, got %v", err)
	}
}

func TestListIssuesTextEscaping(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)
	mustIssue(t, s, p.ID, "feature_flag rollout")
	mustIssue(t, s, p.ID, "featureXflag other")

	// "_" must match literally, so only the first issue matches.
	got, err := s.ListIssues(IssueFilter{ProjectID: p.ID, Text: "feature_flag"})
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(got) != 1 || got[0].Title != "feature_flag rollout" {
		t.Errorf("underscore should be literal; got %d matches: %+v", len(got), got)
	}

	// A bare wildcard must not match everything.
	if got, _ := s.ListIssues(IssueFilter{ProjectID: p.ID, Text: "%"}); len(got) != 0 {
		t.Errorf("'%%' search should match nothing literally, got %d", len(got))
	}
}

func TestCreateLinkDuplicateReturnsExistingID(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)
	a := mustIssue(t, s, p.ID, "a")
	b := mustIssue(t, s, p.ID, "b")

	first, err := s.CreateLink(core.IssueLink{SourceID: a.ID, TargetID: b.ID, Type: core.LinkBlocks})
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}
	// Insert an unrelated row so a naive LastInsertId would return the wrong id.
	mustIssue(t, s, p.ID, "c")

	dup, err := s.CreateLink(core.IssueLink{SourceID: a.ID, TargetID: b.ID, Type: core.LinkBlocks})
	if err != nil {
		t.Fatalf("duplicate CreateLink: %v", err)
	}
	if dup.ID != first.ID {
		t.Errorf("duplicate link returned id %d, want existing %d", dup.ID, first.ID)
	}
	if links, _ := s.ListLinks(a.ID); len(links) != 1 {
		t.Errorf("expected a single edge, got %d", len(links))
	}

	// Missing source surfaces a clear ErrInvalid, not a swallowed not-found.
	if _, err := s.CreateLink(core.IssueLink{SourceID: 99999, TargetID: b.ID, Type: core.LinkBlocks}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("link from missing source: expected ErrInvalid, got %v", err)
	}
}

func TestLinkEventsNotifyBothEndpoints(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)
	a := mustIssue(t, s, p.ID, "a")
	b := mustIssue(t, s, p.ID, "b")

	ch, cancel := s.Subscribe()
	defer cancel()

	link, err := s.CreateLink(core.IssueLink{SourceID: a.ID, TargetID: b.ID, Type: core.LinkBlocks})
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}
	assertNotifiedBoth(t, collectEvents(t, ch, 2), a.ID, b.ID)

	if err := s.DeleteLink(link.ID); err != nil {
		t.Fatalf("DeleteLink: %v", err)
	}
	assertNotifiedBoth(t, collectEvents(t, ch, 2), a.ID, b.ID)
}

func TestGetIssueByKeyErrors(t *testing.T) {
	s := newTestStore(t)
	mustProject(t, s)
	if _, err := s.GetIssueByKey("JEE-999"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown key: expected ErrNotFound, got %v", err)
	}
	if _, err := s.GetIssueByKey("garbage"); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("malformed key: expected ErrInvalid, got %v", err)
	}
}

// --- helpers -----------------------------------------------------------------

func collectEvents(t *testing.T, ch <-chan core.Event, n int) []core.Event {
	t.Helper()
	out := make([]core.Event, 0, n)
	for i := 0; i < n; i++ {
		select {
		case ev := <-ch:
			out = append(out, ev)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for event %d of %d", i+1, n)
		}
	}
	return out
}

func assertNotifiedBoth(t *testing.T, evs []core.Event, a, b int64) {
	t.Helper()
	seen := map[int64]bool{}
	for _, ev := range evs {
		if ev.Type != core.EventIssueUpdated {
			t.Errorf("unexpected event type %q", ev.Type)
		}
		seen[ev.IssueID] = true
	}
	if !seen[a] || !seen[b] {
		t.Errorf("expected events for both %d and %d, saw %v", a, b, seen)
	}
}
