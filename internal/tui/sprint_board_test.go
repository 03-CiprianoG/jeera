package tui

import (
	"strings"
	"testing"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/store"
)

// TestGoldenBoardNoActiveSprint pins the board's empty state: a project with no
// running sprint shows the "start a sprint" prompt rather than bare lanes.
func TestGoldenBoardNoActiveSprint(t *testing.T) {
	m, st := newTestModel(t)
	seedProject(t, st)
	m.reload()
	if m.board.sprint != nil {
		t.Fatalf("expected no active sprint, got %+v", m.board.sprint)
	}
	goldenFile(t, "board_no_active_sprint", render(m))
}

// TestBoardNoSprintEnterOpensSprints checks the empty state's one action: with no
// active sprint, enter (or n) on the board jumps to the Sprints view, where a
// sprint is started or planned.
func TestBoardNoSprintEnterOpensSprints(t *testing.T) {
	m, st := newTestModel(t)
	seedProject(t, st)
	m.reload()
	if m.board.sprint != nil {
		t.Fatal("expected no active sprint")
	}
	next, _ := m.updateBoard(keyPress("enter"))
	if got := next.(Model).view; got != viewSprints {
		t.Errorf("enter on the no-sprint board should open Sprints, got view %v", got)
	}
}

// TestBoardScopedToActiveSprint proves the board is a SCRUM board: it shows only
// the active sprint's issues, hiding both a future sprint's work and the backlog's.
func TestBoardScopedToActiveSprint(t *testing.T) {
	m, st := newTestModel(t)
	p := seedProject(t, st)
	active, _ := st.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "Now", State: core.SprintActive})
	future, _ := st.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "Later", State: core.SprintFuture})

	here, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "in the sprint"})
	if err := st.AddIssueToSprint(here.ID, &active.ID); err != nil {
		t.Fatal(err)
	}
	planned, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "planned for later"})
	if err := st.AddIssueToSprint(planned.ID, &future.ID); err != nil {
		t.Fatal(err)
	}
	st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "still in the backlog"}) // unsprinted

	m.reload()

	if m.board.sprint == nil || m.board.sprint.ID != active.ID {
		t.Fatalf("board should scope to the active sprint, got %+v", m.board.sprint)
	}
	if got := countCards(m.board); got != 1 {
		t.Fatalf("board should show only the active sprint's issue, got %d cards", got)
	}
	if title := m.board.columns[0].cards[0].Title; title != "in the sprint" {
		t.Errorf("board shows the wrong issue: %q", title)
	}
}

// TestBoardCreateJoinsActiveSprint covers the form threading: an issue created
// from the board's add slot joins the active sprint, so it lands on the board
// rather than disappearing into the backlog.
func TestBoardCreateJoinsActiveSprint(t *testing.T) {
	m, st := newTestModel(t)
	p := seedProject(t, st)
	sid := activateSprint(t, st, p.ID)
	m.reload()

	m.colIdx, m.cardIdx = 0, 0 // the To Do add slot (the sprint starts empty)
	if !m.onAddCard() {
		t.Fatalf("expected the To Do add slot")
	}
	next, _ := m.updateBoard(keyPress("enter"))
	m = next.(Model)
	if m.form == nil {
		t.Fatalf("enter on the add slot should open the create form")
	}
	m.form.fields[0].SetValue("born on the board")
	next, _ = m.submitForm()
	m = next.(Model)

	inSprint, _ := st.ListIssues(store.IssueFilter{ProjectID: p.ID, SprintID: &sid})
	if len(inSprint) != 1 || inSprint[0].Title != "born on the board" {
		t.Errorf("a board-created issue should join the active sprint, got %+v", inSprint)
	}
	if got := countCards(m.board); got != 1 {
		t.Errorf("the new issue should appear on the board, got %d cards", got)
	}
}

// TestBoardEmptyStateWhenSprintCompleted guards the live-completion path: when a
// sprint finishes under the board (an agent, or the Sprints view), the board
// falls back to the empty state and the selection clamps away without a panic.
func TestBoardEmptyStateWhenSprintCompleted(t *testing.T) {
	m, st := newTestModel(t)
	p := seedProject(t, st)
	sid := activateSprint(t, st, p.ID)
	wip, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "wip"})
	if err := st.AddIssueToSprint(wip.ID, &sid); err != nil {
		t.Fatal(err)
	}
	m.reload()
	m.colIdx, m.cardIdx = 0, 0
	if _, ok := m.selectedIssue(); !ok {
		t.Fatal("expected a selectable card on the active sprint board")
	}

	if err := st.CompleteSprint(sid); err != nil {
		t.Fatalf("CompleteSprint: %v", err)
	}
	next, _ := m.Update(storeEventMsg{ev: core.Event{Type: core.EventSprintChanged}})
	m = next.(Model)

	if m.board.sprint != nil {
		t.Errorf("board should have no active sprint after completion, got %+v", m.board.sprint)
	}
	if _, ok := m.selectedIssue(); ok {
		t.Error("selection should clamp away when the board empties")
	}
	if !strings.Contains(render(m), "No active sprint") {
		t.Error("board should show the no-active-sprint empty state")
	}
}

// TestCycleStartBlockedBySecondActive checks the one-active-sprint rule reaches
// the TUI: starting a future sprint while one is running surfaces an error and
// leaves the sprint untouched.
func TestCycleStartBlockedBySecondActive(t *testing.T) {
	m, st := newTestModel(t)
	p := seedProject(t, st)
	st.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "running", State: core.SprintActive})
	future, _ := st.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "waiting", State: core.SprintFuture})

	next, cmd := m.cycleSprintState(future)
	m = next.(Model)
	if cmd != nil { // deliver the reported error to the status bar
		if msg := cmd(); msg != nil {
			n2, _ := m.Update(msg)
			m = n2.(Model)
		}
	}
	if m.errText == "" {
		t.Error("starting a second active sprint should surface an error")
	}
	if reloaded, _ := st.GetSprint(future.ID); reloaded.State != core.SprintFuture {
		t.Errorf("the blocked sprint should stay future, got %q", reloaded.State)
	}
}

// TestFilterBoardPreservesSprint guards the search path: filtering the board must
// keep the active-sprint pointer, so the sprint header never blanks mid-search.
func TestFilterBoardPreservesSprint(t *testing.T) {
	sp := &core.Sprint{ID: 7, Name: "S", State: core.SprintActive}
	bd := boardData{
		sprint: sp,
		columns: []column{{
			status: core.Status{Name: "To Do"},
			cards:  []core.Issue{{Key: "JEE-1", Title: "alpha"}, {Key: "JEE-2", Title: "beta"}},
		}},
	}
	out := filterBoard(bd, "alpha")
	if out.sprint != sp {
		t.Fatal("filterBoard must preserve the active sprint pointer")
	}
	if got := countCards(out); got != 1 {
		t.Errorf("filter should narrow to the matching card, got %d", got)
	}
}
