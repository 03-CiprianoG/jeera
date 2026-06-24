package tui

import (
	"testing"

	"github.com/03-CiprianoG/jeera/internal/core"
)

func TestSprintCreateViaForm(t *testing.T) {
	m, st := newTestModel(t)
	p := seedProject(t, st)
	m.reload()
	m.view = viewSprints

	next, _ := m.Update(keyPress("n"))
	m = next.(Model)
	if m.mode != modeForm || m.form == nil || m.form.kind != formCreateSprint {
		t.Fatalf("n in the Sprints view should open the new-sprint form, mode=%v", m.mode)
	}

	m.form.fields[0].SetValue("Q3 push")
	next, _ = m.submitForm()
	m = next.(Model)

	sprints, _ := st.ListSprints(p.ID)
	if len(sprints) != 1 || sprints[0].Name != "Q3 push" {
		t.Fatalf("submitting should create the sprint, got %+v", sprints)
	}
	if sprints[0].State != core.SprintFuture {
		t.Errorf("a new sprint should default to future, got %s", sprints[0].State)
	}
}

func TestSprintCreateRequiresName(t *testing.T) {
	m, st := newTestModel(t)
	p := seedProject(t, st)
	m.reload()
	m.form = newCreateSprintForm()
	m.mode = modeForm // empty name

	next, _ := m.submitForm()
	m = next.(Model)
	if m.mode != modeForm {
		t.Error("an empty sprint name should keep the form open")
	}
	if sprints, _ := st.ListSprints(p.ID); len(sprints) != 0 {
		t.Errorf("no sprint should have been created, got %d", len(sprints))
	}
}

func TestSprintCycleAllStates(t *testing.T) {
	for _, tc := range []struct{ from, to core.SprintState }{
		{core.SprintFuture, core.SprintActive},
		{core.SprintActive, core.SprintCompleted},
		{core.SprintCompleted, core.SprintFuture},
	} {
		m, st := newTestModel(t)
		p := seedProject(t, st)
		sp, _ := st.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "S", State: tc.from})
		m.reload()
		m.view = viewSprints
		m.refreshView()
		m.sprintSel = 0 // the sprint header

		next, _ := m.Update(keyPress("s"))
		m = next.(Model)
		if got, _ := st.GetSprint(sp.ID); got.State != tc.to {
			t.Errorf("cycle from %s: got %s, want %s", tc.from, got.State, tc.to)
		}
	}
}

func TestSprintCycleIgnoresIssueRow(t *testing.T) {
	m, st := newTestModel(t)
	seedSprints(t, &m)
	m.sprintSel = 1 // an issue row, not a header
	it, _ := m.selectedSprintItem()
	before, _ := st.GetSprint(it.sprint.ID)

	next, _ := m.Update(keyPress("s"))
	m = next.(Model)
	if after, _ := st.GetSprint(it.sprint.ID); after.State != before.State {
		t.Errorf("s on an issue row must not change its sprint state: %s → %s", before.State, after.State)
	}
}

func TestSprintDeleteConfirms(t *testing.T) {
	m, st := newTestModel(t)
	seedSprints(t, &m)
	m.sprintSel = 0 // Sprint 1 header
	it, _ := m.selectedSprintItem()
	id := it.sprint.ID

	next, _ := m.Update(keyPress("x"))
	m = next.(Model)
	if m.mode != modeConfirm {
		t.Fatalf("x on a sprint header should ask to confirm, mode=%v", m.mode)
	}
	next, _ = m.updateConfirm(keyPress("y"))
	m = next.(Model)
	if _, err := st.GetSprint(id); err == nil {
		t.Error("confirming should delete the sprint")
	}
}

func TestSprintDeleteCancelKeepsSprint(t *testing.T) {
	m, st := newTestModel(t)
	seedSprints(t, &m)
	m.sprintSel = 0
	it, _ := m.selectedSprintItem()

	next, _ := m.Update(keyPress("x"))
	m = next.(Model)
	next, _ = m.updateConfirm(keyPress("n")) // decline
	m = next.(Model)
	if _, err := st.GetSprint(it.sprint.ID); err != nil {
		t.Error("declining the confirm should keep the sprint")
	}
}

func TestSprintCreatePersistsGoal(t *testing.T) {
	m, st := newTestModel(t)
	p := seedProject(t, st)
	m.reload()
	m.form = newCreateSprintForm()
	m.form.fields[0].SetValue("Sprint X")
	m.form.fields[1].SetValue("Ship the redesign")
	m.mode = modeForm

	next, _ := m.submitForm()
	m = next.(Model)
	sprints, _ := st.ListSprints(p.ID)
	if len(sprints) != 1 || sprints[0].Goal != "Ship the redesign" {
		t.Errorf("sprint goal not persisted: %+v", sprints)
	}
}

func TestGoldenSprintsIssueSelected(t *testing.T) {
	m, _ := newTestModel(t)
	seedSprints(t, &m)
	m.sprintSel = 1 // an issue row — locks in the issue-context footer (open/move/⌫)
	goldenFile(t, "sprints_issue_selected", render(m))
}

func TestSprintUnsprintReturnsIssueToBacklog(t *testing.T) {
	m, st := newTestModel(t)
	seedSprints(t, &m)
	m.sprintSel = 1 // JEE-1, an issue in Sprint 1
	iss, ok := m.selectedSprintIssue()
	if !ok {
		t.Fatal("expected an issue at row 1")
	}

	next, _ := m.Update(keyPress("backspace"))
	m = next.(Model)
	got, _ := st.GetIssue(iss.ID)
	if got.SprintID != nil {
		t.Errorf("backspace should return the issue to the backlog, sprint = %v", got.SprintID)
	}
}

func TestSprintAddIssueOpensBacklogPicker(t *testing.T) {
	m, _ := newTestModel(t)
	seedSprints(t, &m) // every seeded issue is already in a sprint
	// Give the project one unsprinted issue to offer.
	m.store.CreateIssue(core.Issue{ProjectID: m.active.ID, Title: "Loose end", Type: core.TypeTask})

	m.sprintSel = 0 // Sprint 1 header
	next, _ := m.Update(keyPress("a"))
	m = next.(Model)
	if m.mode != modePicker || m.picker == nil {
		t.Fatalf("a on a sprint header should open the add-issue picker, mode=%v", m.mode)
	}
	if len(m.picker.items) != 1 {
		t.Errorf("picker should list the 1 backlog issue, got %d", len(m.picker.items))
	}
}

func TestSprintMoveIssueOpensSprintPicker(t *testing.T) {
	m, _ := newTestModel(t)
	seedSprints(t, &m)
	m.sprintSel = 1 // JEE-1, currently in Sprint 1
	next, _ := m.Update(keyPress("a"))
	m = next.(Model)
	if m.mode != modePicker || m.picker == nil {
		t.Fatalf("a on an issue should open the sprint picker to move it")
	}
	// The current sprint is excluded, so only the other sprint is offered.
	if len(m.picker.items) != 1 {
		t.Errorf("picker should offer the 1 other sprint, got %d", len(m.picker.items))
	}
}

func TestPickerCancelLeavesNothingChanged(t *testing.T) {
	m, st := newTestModel(t)
	seedSprints(t, &m)
	m.sprintSel = 1
	iss, _ := m.selectedSprintIssue()
	before := iss.SprintID

	next, _ := m.Update(keyPress("a")) // open the move picker
	m = next.(Model)
	next, _ = m.Update(keyPress("esc")) // cancel
	m = next.(Model)
	if m.mode != modeNormal || m.picker != nil {
		t.Errorf("esc should close the picker, mode=%v", m.mode)
	}
	got, _ := st.GetIssue(iss.ID)
	if (got.SprintID == nil) != (before == nil) {
		t.Error("cancelling the picker should not move the issue")
	}
}
