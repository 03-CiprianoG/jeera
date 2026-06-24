package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// seedActivePlusEmpty builds an active sprint with two issues and an empty future
// sprint, exercising the no-issues render branch and the flat-cursor walk across
// an empty sprint.
func seedActivePlusEmpty(t *testing.T, m *Model) {
	t.Helper()
	st := m.store
	p := seedProject(t, st)
	active, err := st.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "Sprint 1", State: core.SprintActive})
	if err != nil {
		t.Fatalf("CreateSprint: %v", err)
	}
	if _, err := st.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "Sprint 2", State: core.SprintFuture}); err != nil {
		t.Fatalf("CreateSprint: %v", err)
	}
	ia, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "First issue", Type: core.TypeTask})
	ib, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "Second issue", Type: core.TypeTask})
	sid := active.ID
	if err := st.AddIssueToSprint(ia.ID, &sid); err != nil {
		t.Fatal(err)
	}
	if err := st.AddIssueToSprint(ib.ID, &sid); err != nil {
		t.Fatal(err)
	}
	m.reload()
	m.view = viewSprints
	m.refreshView()
}

func TestSprintsLiveReanchorsCursor(t *testing.T) {
	m, st := newTestModel(t)
	seedSprints(t, &m)
	flat := m.sprints.flatIssues() // [JEE-1, JEE-2, JEE-3], active sprint leading
	if len(flat) != 3 {
		t.Fatalf("expected 3 issues across sprints, got %d", len(flat))
	}
	m.sprintSel = 1 // the middle issue
	want := flat[1].ID

	// An issue ABOVE the selection is deleted while the Sprints view is open, so the
	// flat list reindexes. Re-anchoring must keep the SAME issue selected — a plain
	// clamp would leave the cursor on a different one.
	if err := st.DeleteIssue(flat[0].ID); err != nil {
		t.Fatalf("DeleteIssue: %v", err)
	}
	next, _ := m.Update(storeEventMsg{})
	m = next.(Model)

	got, ok := m.selectedSprintIssue()
	if !ok || got.ID != want {
		t.Errorf("cursor drifted off the selected issue: got %d (ok=%v), want %d", got.ID, ok, want)
	}
}

func TestSprintsLiveReloadClampsOnShrink(t *testing.T) {
	m, st := newTestModel(t)
	seedActivePlusEmpty(t, &m) // two issues
	m.sprintSel = 1            // the last one

	flat := m.sprints.flatIssues()
	if err := st.DeleteIssue(flat[len(flat)-1].ID); err != nil {
		t.Fatal(err)
	}
	next, _ := m.Update(storeEventMsg{})
	m = next.(Model)

	if n := len(m.sprints.flatIssues()); n != 1 {
		t.Errorf("flat list should have shrunk to 1, got %d", n)
	}
	if m.sprintSel != 0 {
		t.Errorf("after the last issue vanished, cursor = %d, want 0", m.sprintSel)
	}
}

func TestGoldenSprintsWithEmptySprint(t *testing.T) {
	m, _ := newTestModel(t)
	seedActivePlusEmpty(t, &m)
	m.sprintSel = 0
	out := render(m)
	if !strings.Contains(out, "— no issues —") {
		t.Error("an empty sprint should render its no-issues row")
	}
	goldenFile(t, "sprints_empty_sprint", out)
}

func TestSprintsEmptySprintAddsNoSelectableRow(t *testing.T) {
	m, _ := newTestModel(t)
	seedActivePlusEmpty(t, &m) // the active sprint has two issues; the future sprint is empty
	if n := len(m.sprints.flatIssues()); n != 2 {
		t.Fatalf("an empty sprint must not add selectable issues, flat = %d", n)
	}
	m.sprintSel = 1 // the last real issue
	next, _ := m.Update(keyPress("j"))
	if got := next.(Model).sprintSel; got != 1 {
		t.Errorf("down past the last issue should clamp (the empty sprint adds no row), got %d", got)
	}
}

func TestLoadSprintsOrdering(t *testing.T) {
	_, st := newTestModel(t)
	p := seedProject(t, st)
	for _, sp := range []core.Sprint{ // scrambled creation order, includes a completed one
		{ProjectID: p.ID, Name: "done", State: core.SprintCompleted},
		{ProjectID: p.ID, Name: "live-1", State: core.SprintActive},
		{ProjectID: p.ID, Name: "next", State: core.SprintFuture},
		{ProjectID: p.ID, Name: "live-2", State: core.SprintActive},
	} {
		if _, err := st.CreateSprint(sp); err != nil {
			t.Fatalf("CreateSprint: %v", err)
		}
	}

	sd, err := loadSprints(st, p.ID)
	if err != nil {
		t.Fatalf("loadSprints: %v", err)
	}
	var got []core.SprintState
	for _, r := range sd.sprints {
		got = append(got, r.sprint.State)
	}
	want := []core.SprintState{core.SprintActive, core.SprintActive, core.SprintFuture, core.SprintCompleted}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sprint order = %v, want active→future→completed %v", got, want)
	}
}

func TestSprintStateOrder(t *testing.T) {
	if !(sprintStateOrder(core.SprintActive) < sprintStateOrder(core.SprintFuture) &&
		sprintStateOrder(core.SprintFuture) < sprintStateOrder(core.SprintCompleted)) {
		t.Errorf("ranking should be active < future < completed, got %d/%d/%d",
			sprintStateOrder(core.SprintActive), sprintStateOrder(core.SprintFuture), sprintStateOrder(core.SprintCompleted))
	}
}

func TestSprintsClampSel(t *testing.T) {
	m, _ := newTestModel(t)
	seedSprints(t, &m) // three issues
	m.sprintSel = 9
	m.clampSprintSel()
	if m.sprintSel != 2 {
		t.Errorf("out-of-range clamp = %d, want last index 2", m.sprintSel)
	}
	m.sprints = sprintsData{}
	m.sprintSel = 5
	m.clampSprintSel()
	if m.sprintSel != 0 {
		t.Errorf("clamp on an empty list = %d, want 0", m.sprintSel)
	}
}

func TestSprintsEnterOnEmptyIsNoop(t *testing.T) {
	m, _ := newTestModel(t)
	seedProject(t, m.store)
	m.reload()
	m.view = viewSprints
	m.refreshView()
	if _, ok := m.selectedSprintIssue(); ok {
		t.Fatal("nothing should be selectable with no sprints")
	}
	next, _ := m.Update(keyPress("enter"))
	m = next.(Model)
	if m.mode != modeNormal || m.detail != nil {
		t.Errorf("enter with nothing selected should be a no-op, mode=%v detail=%v", m.mode, m.detail)
	}
}

func TestSprintsScrollKeepsSelectionVisible(t *testing.T) {
	m, _ := newTestModel(t)
	seedSprints(t, &m)
	m.height = 9 // a short terminal forces the sprint list to scroll
	m.sprintSel = len(m.sprints.flatIssues()) - 1
	last, _ := m.selectedSprintIssue()

	out := render(m)
	if !strings.Contains(out, last.Key) {
		t.Errorf("the selected issue %s must stay on screen when the list scrolls", last.Key)
	}
	if strings.Contains(out, "Sprint 1") {
		t.Error("the first sprint header should have scrolled off to keep the selection visible")
	}
}
