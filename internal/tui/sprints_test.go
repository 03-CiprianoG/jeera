package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/03-CiprianoG/jeera/internal/core"
)

func mustParseDay(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return d
}

// seedSprints gives the model an active sprint with two issues (one estimated)
// and a future sprint with one, so the Sprints view's grouping, state colour,
// counts and points are all exercised. Dates are left unset so the golden stays
// timezone-independent (sprintDates is covered separately).
func seedSprints(t *testing.T, m *Model) {
	t.Helper()
	st := m.store
	p := seedProject(t, st)

	active, err := st.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "Sprint 1", Goal: "Ship the board", State: core.SprintActive})
	if err != nil {
		t.Fatalf("CreateSprint: %v", err)
	}
	future, err := st.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "Sprint 2", State: core.SprintFuture})
	if err != nil {
		t.Fatalf("CreateSprint: %v", err)
	}

	pts := 5
	a, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "Design the kanban board layout", Type: core.TypeStory, Priority: core.PriorityHigh, StoryPoints: &pts})
	b, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "Wire up the MCP status pill", Type: core.TypeTask, Priority: core.PriorityMedium})
	c, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "Plan the sprints view", Type: core.TypeTask, Priority: core.PriorityLow})
	for _, pair := range []struct {
		id     int64
		sprint int64
	}{{a.ID, active.ID}, {b.ID, active.ID}, {c.ID, future.ID}} {
		sid := pair.sprint
		if err := st.AddIssueToSprint(pair.id, &sid); err != nil {
			t.Fatalf("AddIssueToSprint: %v", err)
		}
	}

	m.reload()
	m.view = viewSprints
	m.refreshView()
}

func TestGoldenSprints(t *testing.T) {
	m, _ := newTestModel(t)
	seedSprints(t, &m)
	m.sprintSel = 0
	goldenFile(t, "sprints", render(m))
}

func TestGoldenSprintsEmpty(t *testing.T) {
	m, _ := newTestModel(t)
	seedProject(t, m.store)
	m.reload()
	m.view = viewSprints
	m.refreshView()
	goldenFile(t, "sprints_empty", render(m))
}

func TestSprintsCursorNavigation(t *testing.T) {
	m, _ := newTestModel(t)
	seedSprints(t, &m) // three issues across two sprints
	m.sprintSel = 0

	step := func(key string) {
		next, _ := m.Update(keyPress(key))
		m = next.(Model)
	}

	step("j")
	if m.sprintSel != 1 {
		t.Fatalf("after down, cursor = %d, want 1", m.sprintSel)
	}
	step("j")
	if m.sprintSel != 2 {
		t.Fatalf("after down, cursor = %d, want 2", m.sprintSel)
	}
	step("j") // clamp at the last issue across all sprints
	if m.sprintSel != 2 {
		t.Fatalf("down past the end should clamp, cursor = %d", m.sprintSel)
	}
	step("k")
	if m.sprintSel != 1 {
		t.Fatalf("after up, cursor = %d, want 1", m.sprintSel)
	}
}

func TestSprintsEnterOpensDetail(t *testing.T) {
	m, _ := newTestModel(t)
	seedSprints(t, &m)
	m.sprintSel = 0

	want, ok := m.selectedSprintIssue()
	if !ok {
		t.Fatal("expected an issue under the cursor")
	}
	next, _ := m.Update(keyPress("enter"))
	m = next.(Model)
	if m.mode != modeDetail || m.detail == nil {
		t.Fatalf("enter should open the issue detail, mode=%v detail=%v", m.mode, m.detail)
	}
	if m.detail.issueID != want.ID {
		t.Errorf("opened issue %d, want the selected %d", m.detail.issueID, want.ID)
	}
}

func TestSprintsEmptyHasNoActionHints(t *testing.T) {
	m, _ := newTestModel(t)
	seedProject(t, m.store)
	m.reload()
	m.view = viewSprints
	m.refreshView()
	out := render(m)
	if !strings.Contains(out, "No sprints yet") {
		t.Error("an empty Sprints view should explain itself")
	}
	if strings.Contains(out, "open") { // the Enter hint reads "enter open"
		t.Error("an empty Sprints view should not offer the open action")
	}
}

func TestSprintDates(t *testing.T) {
	if got := sprintDates(core.Sprint{}); got != "" {
		t.Errorf("no bounds should render no range, got %q", got)
	}
	// With both bounds set the range carries an en-dash; exact dates are local and
	// asserted only structurally so the test is timezone-independent.
	start := mustParseDay(t, "2026-06-23")
	end := mustParseDay(t, "2026-07-04")
	if got := sprintDates(core.Sprint{StartAt: &start, EndAt: &end}); !strings.Contains(got, "–") {
		t.Errorf("a full range should contain an en-dash, got %q", got)
	}
	if got := sprintDates(core.Sprint{StartAt: &start}); !strings.HasPrefix(got, "from ") {
		t.Errorf("a start-only sprint should read 'from …', got %q", got)
	}
}
