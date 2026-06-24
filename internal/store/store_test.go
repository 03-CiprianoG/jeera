package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// newTestStore opens a fresh on-disk database in a temp dir so tests exercise
// the real driver, WAL journaling and foreign-key enforcement (an in-memory DB
// would not behave identically). It is closed automatically when the test ends.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "jeera.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func mustProject(t *testing.T, s *Store) core.Project {
	t.Helper()
	p, err := s.CreateProject(core.Project{Name: "Jeera", KeyPrefix: "JEE", RepoPath: "/tmp/jeera"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return p
}

func mustIssue(t *testing.T, s *Store, projectID int64, title string) core.Issue {
	t.Helper()
	iss, err := s.CreateIssue(core.Issue{ProjectID: projectID, Title: title, Type: core.TypeStory})
	if err != nil {
		t.Fatalf("CreateIssue(%q): %v", title, err)
	}
	return iss
}

func ptr[T any](v T) *T { return &v }

func TestProjectLifecycle(t *testing.T) {
	s := newTestStore(t)
	p, err := s.CreateProject(core.Project{
		Name: "Jeera", KeyPrefix: "jee", RepoPath: "/tmp/jeera",
		Defaults: core.ProjectDefaults{Provider: core.ProviderClaude, Model: "opus", Effort: core.EffortHigh},
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.ID == 0 || p.KeyPrefix != "JEE" {
		t.Fatalf("expected normalized prefix JEE with id, got %+v", p)
	}

	// A new project must be seeded with the three default board columns.
	statuses, err := s.ListStatuses(p.ID)
	if err != nil {
		t.Fatalf("ListStatuses: %v", err)
	}
	if len(statuses) != len(DefaultStatuses()) {
		t.Fatalf("expected %d seeded statuses, got %d", len(DefaultStatuses()), len(statuses))
	}
	if statuses[0].Category != core.CategoryTodo {
		t.Errorf("first column should be a todo column, got %q", statuses[0].Category)
	}

	got, err := s.GetProjectByPrefix("JEE")
	if err != nil || got.ID != p.ID {
		t.Fatalf("GetProjectByPrefix: %v (%+v)", err, got)
	}
	if got.Defaults.Model != "opus" || got.Defaults.Provider != core.ProviderClaude {
		t.Errorf("defaults not persisted: %+v", got.Defaults)
	}

	p.Name = "Jeera v2"
	if err := s.UpdateProject(p); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	if again, _ := s.GetProject(p.ID); again.Name != "Jeera v2" {
		t.Errorf("update not persisted, got %q", again.Name)
	}

	list, _ := s.ListProjects()
	if len(list) != 1 {
		t.Errorf("expected 1 project, got %d", len(list))
	}
}

func TestProjectNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetProject(999); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
	if _, err := s.GetIssue(999); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDuplicatePrefixRejected(t *testing.T) {
	s := newTestStore(t)
	mustProject(t, s)
	if _, err := s.CreateProject(core.Project{Name: "Other", KeyPrefix: "JEE", RepoPath: "/x"}); err == nil {
		t.Fatal("expected duplicate prefix to be rejected by the UNIQUE constraint")
	}
}

func TestIssueSequenceAndKey(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)

	var keys []string
	for i := 0; i < 3; i++ {
		iss := mustIssue(t, s, p.ID, "task")
		keys = append(keys, iss.Key)
		if iss.Seq != int64(i+1) {
			t.Errorf("issue %d: seq = %d, want %d", i, iss.Seq, i+1)
		}
	}
	want := []string{"JEE-1", "JEE-2", "JEE-3"}
	for i := range want {
		if keys[i] != want[i] {
			t.Errorf("key[%d] = %q, want %q", i, keys[i], want[i])
		}
	}

	// Key round-trips through the store.
	got, err := s.GetIssueByKey("jee-2")
	if err != nil {
		t.Fatalf("GetIssueByKey: %v", err)
	}
	if got.Seq != 2 {
		t.Errorf("GetIssueByKey returned seq %d, want 2", got.Seq)
	}

	// A second project keeps an independent sequence.
	p2, _ := s.CreateProject(core.Project{Name: "Web", KeyPrefix: "WEB", RepoPath: "/w"})
	iss := mustIssue(t, s, p2.ID, "first web task")
	if iss.Key != "WEB-1" {
		t.Errorf("second project key = %q, want WEB-1", iss.Key)
	}
}

func TestIssueDefaultsAndNullableRoundTrip(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)

	// Only a title supplied: type/priority/status defaulted.
	iss, err := s.CreateIssue(core.Issue{ProjectID: p.ID, Title: "minimal"})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if iss.Type != core.TypeTask || iss.Priority != core.PriorityMedium {
		t.Errorf("defaults wrong: type=%q priority=%q", iss.Type, iss.Priority)
	}
	statuses, _ := s.ListStatuses(p.ID)
	if iss.StatusID != statuses[0].ID {
		t.Errorf("default status = %d, want first column %d", iss.StatusID, statuses[0].ID)
	}

	// Nullable fields round-trip.
	full := core.Issue{
		ProjectID: p.ID, Title: "full", Type: core.TypeBug, Priority: core.PriorityHigh,
		StoryPoints: ptr(5), WorktreeOn: ptr(true),
		Assignee: core.Assignee{Provider: core.ProviderClaude, Model: "opus", Effort: core.EffortHigh},
	}
	created, err := s.CreateIssue(full)
	if err != nil {
		t.Fatalf("CreateIssue full: %v", err)
	}
	reloaded, err := s.GetIssue(created.ID)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if reloaded.StoryPoints == nil || *reloaded.StoryPoints != 5 {
		t.Errorf("story points round-trip failed: %v", reloaded.StoryPoints)
	}
	if reloaded.WorktreeOn == nil || *reloaded.WorktreeOn != true {
		t.Errorf("worktree_on round-trip failed: %v", reloaded.WorktreeOn)
	}
	if reloaded.Assignee != full.Assignee {
		t.Errorf("assignee round-trip failed: got %+v", reloaded.Assignee)
	}
}

func TestListIssuesFilters(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)
	statuses, _ := s.ListStatuses(p.ID)
	sprint, _ := s.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "S1"})

	a := mustIssue(t, s, p.ID, "alpha widget")
	b := mustIssue(t, s, p.ID, "beta gadget")
	_ = b
	c := mustIssue(t, s, p.ID, "gamma widget")

	// Move c to the second column and assign a to the sprint.
	if err := s.TransitionIssue(c.ID, statuses[1].ID); err != nil {
		t.Fatalf("TransitionIssue: %v", err)
	}
	if err := s.AddIssueToSprint(a.ID, ptr(sprint.ID)); err != nil {
		t.Fatalf("AddIssueToSprint: %v", err)
	}

	all, _ := s.ListIssues(IssueFilter{ProjectID: p.ID})
	if len(all) != 3 {
		t.Fatalf("expected 3 issues, got %d", len(all))
	}

	inFirst, _ := s.ListIssues(IssueFilter{ProjectID: p.ID, StatusID: statuses[0].ID})
	if len(inFirst) != 2 {
		t.Errorf("expected 2 issues in first column, got %d", len(inFirst))
	}

	unsprinted, _ := s.ListIssues(IssueFilter{ProjectID: p.ID, Unsprinted: true})
	if len(unsprinted) != 2 {
		t.Errorf("expected 2 unsprinted issues, got %d", len(unsprinted))
	}

	inSprint, _ := s.ListIssues(IssueFilter{ProjectID: p.ID, SprintID: ptr(sprint.ID)})
	if len(inSprint) != 1 || inSprint[0].ID != a.ID {
		t.Errorf("expected only alpha in sprint, got %+v", inSprint)
	}

	widgets, _ := s.ListIssues(IssueFilter{ProjectID: p.ID, Text: "widget"})
	if len(widgets) != 2 {
		t.Errorf("expected 2 widget matches, got %d", len(widgets))
	}
}

func TestTransitionCrossProjectRejected(t *testing.T) {
	s := newTestStore(t)
	p1 := mustProject(t, s)
	p2, _ := s.CreateProject(core.Project{Name: "Web", KeyPrefix: "WEB", RepoPath: "/w"})
	iss := mustIssue(t, s, p1.ID, "x")
	otherStatuses, _ := s.ListStatuses(p2.ID)

	err := s.TransitionIssue(iss.ID, otherStatuses[0].ID)
	if err == nil || !errors.Is(err, core.ErrInvalid) {
		t.Fatalf("expected ErrInvalid for cross-project transition, got %v", err)
	}
}

func TestCascadeDeletes(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)
	iss := mustIssue(t, s, p.ID, "parent")
	if _, err := s.AddComment(core.Comment{IssueID: iss.ID, Body: "hi"}); err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	// Deleting the issue cascades to its comments (FK ON DELETE CASCADE).
	if err := s.DeleteIssue(iss.ID); err != nil {
		t.Fatalf("DeleteIssue: %v", err)
	}
	if cs, _ := s.ListComments(iss.ID); len(cs) != 0 {
		t.Errorf("expected comments cascade-deleted, got %d", len(cs))
	}

	// Deleting the project cascades to its issues.
	iss2 := mustIssue(t, s, p.ID, "child")
	if err := s.DeleteProject(p.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if _, err := s.GetIssue(iss2.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected project's issues cascade-deleted, got %v", err)
	}
}

func TestTags(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)
	iss := mustIssue(t, s, p.ID, "x")
	tag, err := s.CreateTag(core.Tag{ProjectID: p.ID, Name: "backend", Color: "#7C7CF0"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	if err := s.TagIssue(iss.ID, tag.ID); err != nil {
		t.Fatalf("TagIssue: %v", err)
	}
	// Idempotent: tagging again does not error or duplicate.
	if err := s.TagIssue(iss.ID, tag.ID); err != nil {
		t.Fatalf("TagIssue (again): %v", err)
	}
	tags, _ := s.ListIssueTags(iss.ID)
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	if err := s.UntagIssue(iss.ID, tag.ID); err != nil {
		t.Fatalf("UntagIssue: %v", err)
	}
	if tags, _ := s.ListIssueTags(iss.ID); len(tags) != 0 {
		t.Errorf("expected 0 tags after untag, got %d", len(tags))
	}
}

func TestLinksBidirectional(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)
	a := mustIssue(t, s, p.ID, "a")
	b := mustIssue(t, s, p.ID, "b")

	if _, err := s.CreateLink(core.IssueLink{SourceID: a.ID, TargetID: b.ID, Type: core.LinkBlocks}); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	// From A's side it's "blocks"; from B's side it's the inverse "blocked_by".
	fromA, _ := s.ListLinks(a.ID)
	if len(fromA) != 1 || fromA[0].Type != core.LinkBlocks || fromA[0].Issue.ID != b.ID {
		t.Errorf("from A expected blocks->b, got %+v", fromA)
	}
	fromB, _ := s.ListLinks(b.ID)
	if len(fromB) != 1 || fromB[0].Type != core.LinkBlockedBy || fromB[0].Issue.ID != a.ID {
		t.Errorf("from B expected blocked_by->a, got %+v", fromB)
	}

	if err := s.DeleteLink(fromA[0].LinkID); err != nil {
		t.Fatalf("DeleteLink: %v", err)
	}
	if l, _ := s.ListLinks(a.ID); len(l) != 0 {
		t.Errorf("expected link removed, got %d", len(l))
	}
}

func TestComments(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)
	iss := mustIssue(t, s, p.ID, "x")

	if _, err := s.AddComment(core.Comment{IssueID: iss.ID, Body: "from human"}); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	if _, err := s.AddComment(core.Comment{IssueID: iss.ID, Author: "run:7", Body: "from agent"}); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	cs, _ := s.ListComments(iss.ID)
	if len(cs) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(cs))
	}
	if cs[0].Author != "human" {
		t.Errorf("default author = %q, want human", cs[0].Author)
	}
	if cs[1].Author != "run:7" {
		t.Errorf("agent author = %q, want run:7", cs[1].Author)
	}
}

func TestChangeEventsPublished(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)

	ch, cancel := s.Subscribe()
	defer cancel()

	iss := mustIssue(t, s, p.ID, "watch me")
	ev := waitEvent(t, ch)
	if ev.Type != core.EventIssueCreated || ev.IssueID != iss.ID {
		t.Errorf("expected issue.created for %d, got %+v", iss.ID, ev)
	}

	statuses, _ := s.ListStatuses(p.ID)
	if err := s.TransitionIssue(iss.ID, statuses[1].ID); err != nil {
		t.Fatalf("TransitionIssue: %v", err)
	}
	ev = waitEvent(t, ch)
	if ev.Type != core.EventIssueUpdated {
		t.Errorf("expected issue.updated, got %+v", ev)
	}
}

func waitEvent(t *testing.T, ch <-chan core.Event) core.Event {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for change event")
		return core.Event{}
	}
}
