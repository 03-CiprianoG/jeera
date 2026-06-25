package tui

import (
	"testing"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/store"
)

// --- board: the "+ New issue" slot ------------------------------------------

func TestBoardCursorReachesAddSlot(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m) // To Do holds two cards
	m.colIdx, m.cardIdx = 0, 0

	for i := 0; i < 5; i++ { // walk down past the last card
		next, _ := m.updateBoard(keyPress("j"))
		m = next.(Model)
	}
	if m.cardIdx != 2 {
		t.Fatalf("down should clamp on the add slot (index 2), got %d", m.cardIdx)
	}
	if !m.onAddCard() {
		t.Error("cursor past the last card should be the add slot")
	}
	if _, ok := m.selectedIssue(); ok {
		t.Error("the add slot is not an issue")
	}
}

func TestBoardAddSlotCreatesInThatColumn(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m)
	// In Progress is column 1 with one card → its add slot is cardIdx 1.
	m.colIdx, m.cardIdx = 1, 1
	if !m.onAddCard() {
		t.Fatalf("expected the add slot at col 1 / card 1")
	}
	wantStatus := m.board.columns[1].status.ID

	next, _ := m.updateBoard(keyPress("enter"))
	m = next.(Model)
	if m.mode != modeForm || m.form == nil {
		t.Fatalf("enter on the add slot should open the create form")
	}
	if m.form.statusID != wantStatus {
		t.Errorf("form should target the column status %d, got %d", wantStatus, m.form.statusID)
	}

	m.form.fields[0].SetValue("Born in progress")
	next, _ = m.submitForm()
	m = next.(Model)

	found := false
	for _, c := range m.board.columns[1].cards {
		if c.Title == "Born in progress" {
			found = true
		}
	}
	if !found {
		t.Error("the new issue should land in the targeted column")
	}
}

// The board no longer creates issues from a bare keystroke; n is inert there.
func TestBoardNKeyDoesNotCreate(t *testing.T) {
	m, st := newTestModel(t)
	seedProject(t, st)
	m.reload()
	next, _ := m.updateBoard(keyPress("n"))
	m = next.(Model)
	if m.mode != modeNormal {
		t.Errorf("n on a populated board should do nothing, mode=%v", m.mode)
	}
	if issues, _ := st.ListIssues(store.IssueFilter{}); len(issues) != 0 {
		t.Errorf("n should not create an issue, found %d", len(issues))
	}
}

// --- forms: dates and choices ------------------------------------------------

func TestParseDate(t *testing.T) {
	if d, err := parseDate("  "); err != nil || d != nil {
		t.Errorf("blank date should be (nil,nil), got (%v,%v)", d, err)
	}
	d, err := parseDate("2026-07-01")
	if err != nil || d == nil {
		t.Fatalf("valid date errored: %v", err)
	}
	if d.Year() != 2026 || d.Month() != 7 || d.Day() != 1 {
		t.Errorf("parsed the wrong day: %v", d)
	}
	if _, err := parseDate("07/01/2026"); err == nil {
		t.Error("a non-ISO date should be rejected")
	}
}

func TestSprintFormPersistsDates(t *testing.T) {
	m, st := newTestModel(t)
	p := seedProject(t, st)
	m.reload()
	m.form = newCreateSprintForm()
	m.form.fields[0].SetValue("Q3")
	m.form.fields[2].SetValue("2026-07-01")
	m.form.fields[3].SetValue("2026-07-14")
	m.mode = modeForm

	next, _ := m.submitForm()
	m = next.(Model)
	sprints, _ := st.ListSprints(p.ID)
	if len(sprints) != 1 || sprints[0].StartAt == nil || sprints[0].EndAt == nil {
		t.Fatalf("sprint dates not persisted: %+v", sprints)
	}
}

func TestSprintFormRejectsEndBeforeStart(t *testing.T) {
	m, st := newTestModel(t)
	seedProject(t, st)
	m.reload()
	m.form = newCreateSprintForm()
	m.form.fields[0].SetValue("Backwards")
	m.form.fields[2].SetValue("2026-07-14")
	m.form.fields[3].SetValue("2026-07-01")
	m.mode = modeForm

	next, _ := m.submitForm()
	m = next.(Model)
	if m.mode != modeForm {
		t.Error("an end before the start should keep the form open")
	}
}

func TestIssueFormChoiceCycles(t *testing.T) {
	m, st := newTestModel(t)
	seedProject(t, st)
	m.reload()
	m.form = newCreateIssueForm(0)
	m.mode = modeForm
	m.form.fields[0].SetValue("Pick a type")
	m.form.focus = 1 // the Type choice

	// Types are epic, story, task(default), bug, subtask — one step right is bug.
	next, _ := m.updateForm(keyPress("right"))
	m = next.(Model)
	if got := m.form.fields[1].value(); got != string(core.TypeBug) {
		t.Errorf("type after one step right = %q, want %q", got, core.TypeBug)
	}

	next, _ = m.submitForm()
	m = next.(Model)
	if iss, ok := m.selectedIssue(); !ok || iss.Type != core.TypeBug {
		t.Errorf("created issue type = %v (ok=%v), want bug", iss.Type, ok)
	}
}
