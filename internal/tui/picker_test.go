package tui

import (
	"strings"
	"testing"

	"github.com/03-CiprianoG/jeera/internal/core"
)

func TestGoldenPicker(t *testing.T) {
	m, _ := newTestModel(t)
	seedBacklog(t, &m) // one active sprint exists to assign into
	m.backlogSel = 0
	next, _ := m.Update(keyPress("a")) // open the assign-to-sprint picker
	m = next.(Model)
	if m.mode != modePicker {
		t.Fatalf("expected the picker open, mode=%v", m.mode)
	}
	goldenFile(t, "picker", render(m))
}

func TestPickerNavigationAndClamp(t *testing.T) {
	m, _ := newTestModel(t)
	st := m.store
	p := seedProject(t, st)
	st.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "S1", State: core.SprintActive})
	st.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "S2", State: core.SprintFuture})
	st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "loose end", Type: core.TypeTask})
	m.reload()
	m.view = viewBacklog
	m.refreshView()
	m.backlogSel = 0

	next, _ := m.Update(keyPress("a"))
	m = next.(Model)
	if len(m.picker.items) != 2 {
		t.Fatalf("expected 2 sprints offered, got %d", len(m.picker.items))
	}

	step := func(k string) {
		next, _ := m.Update(keyPress(k))
		m = next.(Model)
	}
	step("j")
	if m.picker.sel != 1 {
		t.Fatalf("down → 1, got %d", m.picker.sel)
	}
	step("j") // clamp at the last
	if m.picker.sel != 1 {
		t.Errorf("down past the end should clamp, got %d", m.picker.sel)
	}
	step("k")
	step("k") // clamp at the top
	if m.picker.sel != 0 {
		t.Errorf("up past the top should clamp, got %d", m.picker.sel)
	}
}

func TestPickerEmptyState(t *testing.T) {
	m, _ := newTestModel(t)
	st := m.store
	p := seedProject(t, st)
	sp, _ := st.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "Only sprint", State: core.SprintActive})
	iss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "x", Type: core.TypeTask})
	st.AddIssueToSprint(iss.ID, &sp.ID)
	m.reload()
	m.view = viewSprints
	m.refreshView()

	m.sprintSel = 1 // the issue; moving it offers no OTHER sprint
	next, _ := m.Update(keyPress("a"))
	m = next.(Model)
	if m.mode != modePicker || len(m.picker.items) != 0 {
		t.Fatalf("expected an empty move picker, items=%d", len(m.picker.items))
	}
	if !strings.Contains(render(m), "No other sprints") {
		t.Error("an empty picker should explain why")
	}
	// Enter on an empty picker is a safe no-op that closes.
	next, _ = m.Update(keyPress("enter"))
	m = next.(Model)
	if m.mode != modeNormal || m.picker != nil {
		t.Errorf("enter on an empty picker should close cleanly, mode=%v", m.mode)
	}
}
