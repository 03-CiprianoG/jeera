package tui

import (
	"strings"
	"testing"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// seedBacklog gives the model two unsprinted issues (the backlog) plus one that
// is already in a sprint, so the view's filter is exercised.
func seedBacklog(t *testing.T, m *Model) {
	t.Helper()
	st := m.store
	p := seedProject(t, st)

	pts := 3
	st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "Add OAuth login", Type: core.TypeStory, Priority: core.PriorityHigh, StoryPoints: &pts})
	st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "Fix the flaky test", Type: core.TypeBug, Priority: core.PriorityMedium})

	planned, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "Already planned", Type: core.TypeTask})
	sp, _ := st.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "Sprint 1", State: core.SprintActive})
	if err := st.AddIssueToSprint(planned.ID, &sp.ID); err != nil {
		t.Fatalf("AddIssueToSprint: %v", err)
	}

	m.reload()
	m.view = viewBacklog
	m.refreshView()
}

func TestGoldenBacklog(t *testing.T) {
	m, _ := newTestModel(t)
	seedBacklog(t, &m)
	m.backlogSel = 0
	goldenFile(t, "backlog", render(m))
}

func TestGoldenBacklogEmpty(t *testing.T) {
	m, _ := newTestModel(t)
	seedProject(t, m.store)
	m.reload()
	m.view = viewBacklog
	m.refreshView()
	goldenFile(t, "backlog_empty", render(m))
}

func TestLoadBacklogExcludesSprinted(t *testing.T) {
	m, _ := newTestModel(t)
	seedBacklog(t, &m)
	if n := len(m.backlog.issues); n != 2 {
		t.Fatalf("backlog should hold only the 2 unsprinted issues, got %d", n)
	}
	for _, iss := range m.backlog.issues {
		if iss.SprintID != nil {
			t.Errorf("issue %s is in a sprint but showed in the backlog", iss.Key)
		}
	}
}

func TestBacklogNavigationClamps(t *testing.T) {
	m, _ := newTestModel(t)
	seedBacklog(t, &m) // two issues
	m.backlogSel = 0

	next, _ := m.Update(keyPress("j"))
	m = next.(Model)
	if m.backlogSel != 1 {
		t.Fatalf("down → 1, got %d", m.backlogSel)
	}
	next, _ = m.Update(keyPress("j")) // clamp at the last
	m = next.(Model)
	if m.backlogSel != 1 {
		t.Errorf("down past the end should clamp, got %d", m.backlogSel)
	}
	next, _ = m.Update(keyPress("k"))
	next, _ = next.(Model).Update(keyPress("k")) // clamp at the first
	if got := next.(Model).backlogSel; got != 0 {
		t.Errorf("up past the start should clamp, got %d", got)
	}
}

func TestBacklogEnterOpensDetail(t *testing.T) {
	m, _ := newTestModel(t)
	seedBacklog(t, &m)
	m.backlogSel = 0
	want, ok := m.selectedBacklogIssue()
	if !ok {
		t.Fatal("expected a selected backlog issue")
	}
	next, _ := m.Update(keyPress("enter"))
	m = next.(Model)
	if m.mode != modeDetail || m.detail == nil || m.detail.issueID != want.ID {
		t.Errorf("enter should open the selected backlog issue %d, got mode=%v", want.ID, m.mode)
	}
}

func TestBacklogNewOpensIssueForm(t *testing.T) {
	m, _ := newTestModel(t)
	seedBacklog(t, &m)
	next, _ := m.Update(keyPress("n"))
	m = next.(Model)
	if m.mode != modeForm || m.form == nil || m.form.kind != formCreateIssue {
		t.Errorf("n in the backlog should open the new-issue form, mode=%v", m.mode)
	}
}

func TestBacklogAssignOpensSprintPicker(t *testing.T) {
	m, _ := newTestModel(t)
	seedBacklog(t, &m) // one active sprint exists to assign into
	m.backlogSel = 0
	next, _ := m.Update(keyPress("a"))
	m = next.(Model)
	if m.mode != modePicker || m.picker == nil {
		t.Fatalf("a should open the assign-to-sprint picker, mode=%v", m.mode)
	}
	if len(m.picker.items) != 1 {
		t.Errorf("picker should list the project's 1 sprint, got %d", len(m.picker.items))
	}
}

func TestBacklogAssignToSprint(t *testing.T) {
	m, st := newTestModel(t)
	seedBacklog(t, &m)
	m.backlogSel = 0
	iss, _ := m.selectedBacklogIssue()

	next, _ := m.Update(keyPress("a")) // open picker
	m = next.(Model)
	next, _ = m.Update(keyPress("enter")) // choose the only sprint
	m = next.(Model)

	got, _ := st.GetIssue(iss.ID)
	if got.SprintID == nil {
		t.Error("assigning from the backlog should put the issue in a sprint")
	}
	if m.mode != modeNormal || m.picker != nil {
		t.Errorf("the picker should close after choosing, mode=%v", m.mode)
	}
}

func TestBacklogClampSel(t *testing.T) {
	m, _ := newTestModel(t)
	seedBacklog(t, &m) // two issues
	m.backlogSel = 9
	m.clampBacklogSel()
	if m.backlogSel != 1 {
		t.Errorf("out-of-range clamp = %d, want last index 1", m.backlogSel)
	}

	m.backlog = backlogData{}
	m.backlogSel = 5
	m.clampBacklogSel()
	if m.backlogSel != 0 {
		t.Errorf("clamp on an empty backlog = %d, want 0", m.backlogSel)
	}
	if _, ok := m.selectedBacklogIssue(); ok {
		t.Error("selectedBacklogIssue should be (zero, false) on an empty backlog")
	}
}

func TestBacklogAssignWithNoSprintsIsToast(t *testing.T) {
	m, _ := newTestModel(t)
	st := m.store
	p := seedProject(t, st)
	st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "lonely", Type: core.TypeTask})
	m.reload()
	m.view = viewBacklog
	m.refreshView()
	if m.backlog.sprintCount != 0 {
		t.Fatalf("expected no sprints, got %d", m.backlog.sprintCount)
	}

	m.backlogSel = 0
	next, cmd := m.Update(keyPress("a"))
	m = next.(Model)
	if m.mode == modePicker {
		t.Error("assign with no sprints should not open an empty picker")
	}
	if cmd == nil {
		t.Error("assign with no sprints should toast guidance, got no command")
	}
	if strings.Contains(render(m), "assign") {
		t.Error("the backlog footer must hide 'assign' when there are no sprints")
	}
}
