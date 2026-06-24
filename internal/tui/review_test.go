package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/store"
)

// TestMoveSelectionSurvivesStoreEvent guards the high-severity fix: after a move
// (or create/rename), the asynchronous store-event reload must keep the
// selection on the affected card rather than clamping it away.
func TestMoveSelectionSurvivesStoreEvent(t *testing.T) {
	m, st := newTestModel(t)
	p := seedProject(t, st)
	iss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "follow me"})
	m.reload()
	m.colIdx, m.cardIdx = 0, 0

	next, _ := m.moveSelected(+1)
	m = next.(Model)

	// Simulate the bridged store event that TransitionIssue published.
	next, _ = m.Update(storeEventMsg{ev: core.Event{Type: core.EventIssueUpdated, IssueID: iss.ID}})
	m = next.(Model)

	sel, ok := m.selectedIssue()
	if !ok || sel.ID != iss.ID {
		t.Fatalf("selection lost after async reload: ok=%v sel=%+v", ok, sel)
	}
	if m.colIdx != 1 {
		t.Errorf("selection should stay on the moved card in column 1, got col %d", m.colIdx)
	}
}

func TestStoreEventTriggersReload(t *testing.T) {
	m, st := newTestModel(t)
	p := seedProject(t, st)
	m.reload()
	if got := len(m.board.columns[0].cards); got != 0 {
		t.Fatalf("expected empty board, got %d cards", got)
	}
	// An agent creates an issue directly in the store…
	st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "filed by an agent"})
	// …and the bridged event makes the board reflect it.
	next, _ := m.Update(storeEventMsg{ev: core.Event{Type: core.EventIssueCreated}})
	m = next.(Model)
	if got := len(m.board.columns[0].cards); got != 1 {
		t.Errorf("board did not refresh on store event, %d cards", got)
	}
}

func TestMoveSelectedAtEdgesIsNoOp(t *testing.T) {
	m, st := newTestModel(t)
	p := seedProject(t, st)
	st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "x"})
	m.reload()
	m.colIdx, m.cardIdx = 0, 0

	// Left at the first column is a no-op (stays in column 0).
	next, _ := m.moveSelected(-1)
	m = next.(Model)
	if len(m.board.columns[0].cards) != 1 || m.colIdx != 0 {
		t.Errorf("move left at edge should be a no-op")
	}
	// Move to the last column, then right again is a no-op.
	next, _ = m.moveSelected(+1)
	m = next.(Model)
	next, _ = m.moveSelected(+1)
	m = next.(Model)
	last := len(m.board.columns) - 1
	next, _ = m.moveSelected(+1)
	m = next.(Model)
	if m.colIdx != last {
		t.Errorf("move right at last column should be a no-op, col=%d", m.colIdx)
	}
}

func TestMoveKeyRoutesToTransition(t *testing.T) {
	m, st := newTestModel(t)
	p := seedProject(t, st)
	iss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "x"})
	m.reload()
	m.colIdx, m.cardIdx = 0, 0

	// Shift+L (key "L") routes to a move-right and the selection follows.
	next, _ := m.updateBoard(keyPress("L"))
	m = next.(Model)
	if m.colIdx != 1 {
		t.Fatalf("L should move the card to column 1, got col %d", m.colIdx)
	}
	if sel, ok := m.selectedIssue(); !ok || sel.ID != iss.ID {
		t.Errorf("selection should follow the moved card")
	}
}

func TestConfirmCancelLeavesIssueIntact(t *testing.T) {
	m, st := newTestModel(t)
	p := seedProject(t, st)
	iss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "keep me"})
	m.reload()
	m.colIdx, m.cardIdx = 0, 0

	next, _ := m.updateBoard(keyPress("x"))
	m = next.(Model)
	next, _ = m.updateConfirm(keyPress("n")) // cancel
	m = next.(Model)

	if _, err := st.GetIssue(iss.ID); err != nil {
		t.Errorf("cancelling delete should leave the issue intact: %v", err)
	}
	if m.mode != modeNormal {
		t.Errorf("expected board mode after cancel, got %v", m.mode)
	}
}

func TestFormEscCancelsWithoutMutating(t *testing.T) {
	m, st := newTestModel(t)
	seedProject(t, st)
	m.reload()

	next, _ := m.updateBoard(keyPress("n")) // open create-issue form
	m = next.(Model)
	if m.mode != modeForm {
		t.Fatalf("expected form mode, got %v", m.mode)
	}
	m.form.fields[0].SetValue("should not persist")
	next, _ = m.updateForm(keyPress("esc"))
	m = next.(Model)

	if m.mode != modeNormal || m.form != nil {
		t.Errorf("esc should close the form")
	}
	if issues, _ := st.ListIssues(store.IssueFilter{}); len(issues) != 0 {
		t.Errorf("esc should not create an issue, found %d", len(issues))
	}
}

func TestProjectsPickerNavigateAndSwitch(t *testing.T) {
	m, st := newTestModel(t)
	seedProject(t, st) // JEE, becomes active
	st.CreateProject(core.Project{Name: "Web", KeyPrefix: "WEB", RepoPath: "/w"})
	m.reload()

	m.mode = modeProjects
	m.projSel = m.activeProjectIndex()
	next, _ := m.updateProjects(keyPress("j")) // move selection down
	m = next.(Model)
	next, _ = m.updateProjects(keyPress("enter"))
	m = next.(Model)

	if m.active.KeyPrefix != "WEB" {
		t.Errorf("expected active project WEB after switch, got %s", m.active.KeyPrefix)
	}
	if m.mode != modeNormal {
		t.Errorf("expected board mode after switch, got %v", m.mode)
	}
}

func TestViewNotReadyAndTooSmall(t *testing.T) {
	m, st := newTestModel(t)
	seedProject(t, st)
	m.reload()

	m.width, m.height = 0, 0
	if got := m.View().Content; got != "" {
		t.Errorf("unready view should be empty, got %q", got)
	}
	m.width, m.height = 10, 5
	if got := stripANSI(m.View().Content); !strings.Contains(got, "too small") {
		t.Errorf("tiny view should warn, got %q", got)
	}
}

func TestHeaderFooterStayOneLineWhenNarrow(t *testing.T) {
	m, st := newTestModel(t)
	p, _ := st.CreateProject(core.Project{
		Name: strings.Repeat("Very Long Project Name ", 6), KeyPrefix: "LONG", RepoPath: "/l",
	})
	m.active = p
	m.reload()
	m.width = 40

	if h := lipgloss.Height(m.renderHeader()); h != 1 {
		t.Errorf("header wrapped to %d lines with a long project name", h)
	}
	m.errText = strings.Repeat("an error happened ", 10)
	if h := lipgloss.Height(m.renderFooter()); h != 1 {
		t.Errorf("footer wrapped to %d lines with a long error", h)
	}
}

func TestMCPPillOff(t *testing.T) {
	m, _ := newTestModel(t) // mcp is nil
	if got := stripANSI(m.mcpPill()); !strings.Contains(got, "mcp off") {
		t.Errorf("nil MCP should show 'mcp off', got %q", got)
	}
}

func TestToastAndErrorHandling(t *testing.T) {
	m, _ := newTestModel(t)
	next, _ := m.Update(toastMsg{"saved"})
	m = next.(Model)
	if m.toastText != "saved" {
		t.Errorf("toast not set")
	}
	next, _ = m.Update(errMsg{err: errTest{}})
	m = next.(Model)
	if m.errText == "" || m.toastText != "" {
		t.Errorf("error should set errText and clear toast")
	}
	next, _ = m.Update(clearToastMsg{})
	m = next.(Model)
	if m.toastText != "" {
		t.Errorf("clearToast should clear toast")
	}
}

type errTest struct{}

func (errTest) Error() string { return "boom" }

var _ tea.Msg = storeEventMsg{}
