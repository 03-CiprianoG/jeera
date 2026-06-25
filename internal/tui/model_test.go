package tui

import (
	"path/filepath"
	"testing"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/run"
	"github.com/03-CiprianoG/jeera/internal/schedule"
	"github.com/03-CiprianoG/jeera/internal/store"
)

func TestReloadGroupsIssuesByColumn(t *testing.T) {
	m, st := newTestModel(t)
	p := seedProject(t, st)
	if _, err := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "b"}); err != nil {
		t.Fatal(err)
	}
	m.reload()

	if m.active.ID != p.ID {
		t.Fatalf("active project not selected: %+v", m.active)
	}
	if len(m.board.columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(m.board.columns))
	}
	if got := len(m.board.columns[0].cards); got != 2 {
		t.Errorf("expected 2 cards in first column, got %d", got)
	}
}

func TestMoveSelectedTransitions(t *testing.T) {
	m, st := newTestModel(t)
	p := seedProject(t, st)
	iss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "move me"})
	m.reload()
	m.colIdx, m.cardIdx = 0, 0

	next, _ := m.moveSelected(+1)
	m = next.(Model)

	if len(m.board.columns[1].cards) != 1 {
		t.Fatalf("issue should have moved to column 1, got %d cards", len(m.board.columns[1].cards))
	}
	if m.colIdx != 1 {
		t.Errorf("selection should follow the card to column 1, got %d", m.colIdx)
	}
	sel, ok := m.selectedIssue()
	if !ok || sel.ID != iss.ID {
		t.Errorf("selection lost after move: %+v", sel)
	}
}

func TestSubmitCreateProject(t *testing.T) {
	m, _ := newTestModel(t)
	m.form = newCreateProjectForm()
	m.form.fields[0].SetValue("Web App")
	m.form.fields[1].SetValue("web")
	m.form.fields[2].SetValue("/repos/web-app")
	m.mode = modeForm

	next, _ := m.submitForm()
	m = next.(Model)

	if m.active.KeyPrefix != "WEB" {
		t.Errorf("new project not activated: %+v", m.active)
	}
	if m.active.RepoPath != "/repos/web-app" {
		t.Errorf("repo path not captured: %q", m.active.RepoPath)
	}
	if m.mode != modeNormal {
		t.Errorf("form should close after submit, mode=%v", m.mode)
	}
	if len(m.projects) != 1 {
		t.Errorf("expected 1 project, got %d", len(m.projects))
	}
}

func TestSubmitCreateIssueSelectsResult(t *testing.T) {
	m, st := newTestModel(t)
	seedProject(t, st)
	m.reload()
	m.form = newCreateIssueForm(0)
	m.form.fields[0].SetValue("Ship it")
	m.mode = modeForm

	next, _ := m.submitForm()
	m = next.(Model)

	sel, ok := m.selectedIssue()
	if !ok || sel.Title != "Ship it" {
		t.Errorf("created issue not selected: %+v (ok=%v)", sel, ok)
	}
}

func TestSubmitCreateProjectInvalidKeepsForm(t *testing.T) {
	m, _ := newTestModel(t)
	m.form = newCreateProjectForm()
	m.form.fields[0].SetValue("Bad")
	m.form.fields[1].SetValue("1") // invalid prefix
	m.mode = modeForm

	next, _ := m.submitForm()
	m = next.(Model)

	if m.mode != modeForm || m.form == nil {
		t.Errorf("invalid submit should keep the form open, mode=%v form=%v", m.mode, m.form)
	}
	if len(m.projects) != 0 {
		t.Errorf("no project should have been created, got %d", len(m.projects))
	}
}

func TestRenameViaForm(t *testing.T) {
	m, st := newTestModel(t)
	p := seedProject(t, st)
	iss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "old name"})
	m.reload()

	m.form = newRenameForm(iss)
	m.form.fields[0].SetValue("new name")
	m.mode = modeForm
	next, _ := m.submitForm()
	m = next.(Model)

	reloaded, _ := st.GetIssue(iss.ID)
	if reloaded.Title != "new name" {
		t.Errorf("rename not persisted: %q", reloaded.Title)
	}
}

func TestDeleteFlowViaKeys(t *testing.T) {
	m, st := newTestModel(t)
	p := seedProject(t, st)
	iss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "doomed"})
	m.reload()
	m.colIdx, m.cardIdx = 0, 0

	// x opens the confirm dialog…
	next, _ := m.updateBoard(keyPress("x"))
	m = next.(Model)
	if m.mode != modeConfirm {
		t.Fatalf("expected confirm mode, got %v", m.mode)
	}
	// …and y deletes.
	next, _ = m.updateConfirm(keyPress("y"))
	m = next.(Model)
	if _, err := st.GetIssue(iss.ID); err == nil {
		t.Error("issue should have been deleted")
	}
}

// Deleting a scheduled issue must tear down its live cron job, not just its rows.
func TestDeleteStopsSchedule(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "jeera.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	mgr := run.NewManager(st, t.TempDir(), func() string { return "" }, nil)
	sched, err := schedule.New(st, mgr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sched.Shutdown() })
	if err := sched.Start(); err != nil {
		t.Fatal(err)
	}

	m := New(st, nil, mgr, sched, nil)
	m.width, m.height = 100, 30
	p := seedProject(t, st)
	iss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "doomed but scheduled"})
	sc, _ := sched.Add(iss.ID, "0 9 * * *", false)
	m.reload()
	m.colIdx, m.cardIdx = 0, 0
	if sched.ActiveJobs() != 1 {
		t.Fatalf("expected one live job before delete, got %d", sched.ActiveJobs())
	}

	next, _ := m.updateBoard(keyPress("x"))
	m = next.(Model)
	next, _ = m.updateConfirm(keyPress("y"))
	m = next.(Model)

	if sched.ActiveJobs() != 0 {
		t.Errorf("deleting the issue should stop its cron job, %d remain", sched.ActiveJobs())
	}
	if _, err := st.GetSchedule(sc.ID); err == nil {
		t.Error("schedule row should cascade-delete with the issue")
	}
}

func TestBoardNavigationClamps(t *testing.T) {
	m, st := newTestModel(t)
	p := seedProject(t, st)
	for i := 0; i < 3; i++ {
		st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "x"})
	}
	m.reload()
	m.colIdx, m.cardIdx = 0, 0

	// Up at the top stays at 0.
	next, _ := m.updateBoard(keyPress("k"))
	m = next.(Model)
	if m.cardIdx != 0 {
		t.Errorf("up at top should clamp to 0, got %d", m.cardIdx)
	}
	// Down moves within the column.
	next, _ = m.updateBoard(keyPress("j"))
	m = next.(Model)
	if m.cardIdx != 1 {
		t.Errorf("down should move to 1, got %d", m.cardIdx)
	}
	// Left at the first column clamps.
	next, _ = m.updateBoard(keyPress("h"))
	m = next.(Model)
	if m.colIdx != 0 {
		t.Errorf("left at first column should clamp to 0, got %d", m.colIdx)
	}
	// Right moves to the next column (which is empty → cardIdx clamps to 0).
	next, _ = m.updateBoard(keyPress("l"))
	m = next.(Model)
	if m.colIdx != 1 {
		t.Errorf("right should move to column 1, got %d", m.colIdx)
	}
}
