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
	seedSprints(t, &m) // rows: [hdr S1, JEE-1, JEE-2, hdr S2, JEE-3]
	m.sprintSel = 2    // JEE-2 (an issue; row 0 is the sprint header)
	sel, ok := m.selectedSprintIssue()
	if !ok {
		t.Fatal("expected an issue selected at row 2")
	}
	want := sel.ID

	// Delete an issue ABOVE the selection; the rows reindex. Re-anchoring must keep
	// the SAME issue selected, not whatever slides into row 2.
	flat := m.sprints.flatIssues() // [JEE-1, JEE-2, JEE-3]
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
	seedActivePlusEmpty(t, &m) // rows: [hdr S1, iss1, iss2, hdr S2]
	rowsBefore := len(m.sprints.items())
	m.sprintSel = rowsBefore - 1 // the empty sprint's header (last row)

	flat := m.sprints.flatIssues()
	if err := st.DeleteIssue(flat[len(flat)-1].ID); err != nil {
		t.Fatal(err)
	}
	next, _ := m.Update(storeEventMsg{})
	m = next.(Model)

	if got := len(m.sprints.items()); got != rowsBefore-1 {
		t.Errorf("rows should drop to %d, got %d", rowsBefore-1, got)
	}
	if m.sprintSel >= len(m.sprints.items()) {
		t.Fatalf("cursor %d out of range after shrink (rows=%d)", m.sprintSel, len(m.sprints.items()))
	}
	if it, ok := m.selectedSprintItem(); !ok || it.kind != itemHeader {
		t.Error("cursor should re-anchor to the empty sprint header it was on")
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

func TestSprintsEmptySprintHeaderSelectable(t *testing.T) {
	m, _ := newTestModel(t)
	seedActivePlusEmpty(t, &m) // S1: two issues; S2: empty
	// Rows: [hdr S1, iss1, iss2, hdr S2]. The empty sprint contributes its header
	// (so it can be started/deleted/added to) but no issue rows.
	if got := len(m.sprints.items()); got != 4 {
		t.Fatalf("expected 4 rows, got %d", got)
	}
	if got := len(m.sprints.flatIssues()); got != 2 {
		t.Fatalf("the empty sprint must add no issues, flat = %d", got)
	}
	m.sprintSel = 3 // the empty sprint's header
	it, ok := m.selectedSprintItem()
	if !ok || it.kind != itemHeader {
		t.Fatalf("the empty sprint header should be selectable")
	}
	// Enter on a header opens the full-screen Sprint detail, even for an empty
	// sprint — its goal, window and lifecycle are still worth managing there.
	next, _ := m.Update(keyPress("enter"))
	nm := next.(Model)
	if nm.mode != modeSprintDetail || nm.sprintDetail == nil {
		t.Errorf("enter on a sprint header should open the sprint detail, mode=%v", nm.mode)
	}
	if nm.sprintDetail.sprintID != it.sprint.ID {
		t.Errorf("opened sprint %d, want the selected empty sprint %d", nm.sprintDetail.sprintID, it.sprint.ID)
	}
}

func TestLoadSprintsOrdering(t *testing.T) {
	_, st := newTestModel(t)
	p := seedProject(t, st)
	// Scrambled creation order with a single active sprint (only one is allowed per
	// project), plus two future and one completed, to prove the view sorts
	// active→future→completed while preserving newest-first within each group.
	for _, sp := range []core.Sprint{
		{ProjectID: p.ID, Name: "done", State: core.SprintCompleted},
		{ProjectID: p.ID, Name: "live", State: core.SprintActive},
		{ProjectID: p.ID, Name: "next-1", State: core.SprintFuture},
		{ProjectID: p.ID, Name: "next-2", State: core.SprintFuture},
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
	want := []core.SprintState{core.SprintActive, core.SprintFuture, core.SprintFuture, core.SprintCompleted}
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
	seedSprints(t, &m) // 5 rows (2 headers + 3 issues)
	m.sprintSel = 9
	m.clampSprintSel()
	if m.sprintSel != 4 {
		t.Errorf("out-of-range clamp = %d, want last row 4", m.sprintSel)
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
	m.sprintSel = len(m.sprints.items()) - 1
	last, _ := m.selectedSprintIssue()

	out := render(m)
	if !strings.Contains(out, last.Key) {
		t.Errorf("the selected issue %s must stay on screen when the list scrolls", last.Key)
	}
	if strings.Contains(out, "Sprint 1") {
		t.Error("the first sprint header should have scrolled off to keep the selection visible")
	}
}
