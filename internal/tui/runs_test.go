package tui

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/run"
	"github.com/03-CiprianoG/jeera/internal/store"
)

// newModelWithManager builds a model wired to a real run manager (newTestModel
// passes nil), so the resume action's ResumeCommand path can be exercised.
func newModelWithManager(t *testing.T) (Model, *store.Store, *run.Manager) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "jeera.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	mgr := run.NewManager(st, t.TempDir(), nil, nil)
	m := New(st, nil, mgr, nil, nil)
	m.width, m.height = 100, 30
	return m, st, mgr
}

// seedRuns gives the model a couple of recent runs: a claude run with a session
// id (resumable) and a later codex run without one (its agent died before
// announcing a thread). StartedAt is left nil so the rendered timestamp column
// stays empty and the golden file is timezone-independent.
func seedRuns(t *testing.T, m *Model) {
	t.Helper()
	st := m.store
	p := seedProject(t, st)
	iss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "Resume me", Type: core.TypeTask})
	if _, err := st.CreateRun(core.Run{
		IssueID: iss.ID, Version: 1, Provider: core.ProviderClaude, Model: "opus",
		SessionID: "93049003-b89c", Status: core.RunSucceeded,
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := st.CreateRun(core.Run{
		IssueID: iss.ID, Version: 2, Provider: core.ProviderCodex,
		Status: core.RunFailed, // no session id captured
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	m.reload()
	m.recentRuns, _ = st.ListRecentRuns(50)
}

func TestGoldenRuns(t *testing.T) {
	m, _ := newTestModel(t)
	seedRuns(t, &m)
	m.view = viewRuns
	m.runsCursor = 0 // newest run (codex v2, no session) selected
	goldenFile(t, "runs", render(m))
}

func TestRunsCursorNavigation(t *testing.T) {
	m, _ := newTestModel(t)
	seedRuns(t, &m) // two runs
	m.view = viewRuns

	step := func(key string) {
		next, _ := m.Update(keyPress(key))
		m = next.(Model)
	}

	step("j")
	if m.runsCursor != 1 {
		t.Fatalf("after down, cursor = %d, want 1", m.runsCursor)
	}
	step("j") // clamp at the last row
	if m.runsCursor != 1 {
		t.Fatalf("down past the end should clamp, cursor = %d", m.runsCursor)
	}
	step("k")
	if m.runsCursor != 0 {
		t.Fatalf("after up, cursor = %d, want 0", m.runsCursor)
	}
	step("k") // clamp at the first row
	if m.runsCursor != 0 {
		t.Fatalf("up past the start should clamp, cursor = %d", m.runsCursor)
	}
}

func TestRunsTabReturnsToBoard(t *testing.T) {
	m, _ := newTestModel(t)
	seedRuns(t, &m)
	m.view = viewRuns
	// Runs is a peer view now, not a closeable overlay: Tab cycles
	// board→sprints→runs→board, so from Runs it wraps back to the board.
	next, _ := m.Update(keyPress("tab"))
	if got := next.(Model).view; got != viewBoard {
		t.Errorf("tab from Runs should wrap to the board, got view %d", got)
	}
}

func TestRunsResumeWithoutManagerErrors(t *testing.T) {
	m, _ := newTestModel(t) // newTestModel wires a nil run manager
	seedRuns(t, &m)
	m.view = viewRuns
	_, cmd := m.Update(keyPress("t"))
	if cmd == nil {
		t.Fatal("t should produce a command even when it cannot resume")
	}
	if _, ok := cmd().(errMsg); !ok {
		t.Error("resuming with no run manager should surface an errMsg")
	}
}

// resumableSetup gives a model (with a real manager) one run carrying a session
// id, in an existing working directory so the launch can actually spawn.
func resumableSetup(t *testing.T) (Model, *store.Store) {
	t.Helper()
	m, st, _ := newModelWithManager(t)
	p, _ := st.CreateProject(core.Project{Name: "Jeera", KeyPrefix: "JEE", RepoPath: t.TempDir()})
	iss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "Resume me", Type: core.TypeTask})
	if _, err := st.CreateRun(core.Run{
		IssueID: iss.ID, Version: 1, Provider: core.ProviderClaude,
		SessionID: "sess-abc", WorktreePath: t.TempDir(), Status: core.RunSucceeded,
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	m.reload()
	m.recentRuns, _ = st.ListRecentRuns(50)
	m.view = viewRuns
	return m, st
}

func TestRunsResumeNoSessionErrors(t *testing.T) {
	m, st, _ := newModelWithManager(t)
	p, _ := st.CreateProject(core.Project{Name: "Jeera", KeyPrefix: "JEE", RepoPath: t.TempDir()})
	iss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "x", Type: core.TypeTask})
	st.CreateRun(core.Run{IssueID: iss.ID, Version: 1, Provider: core.ProviderClaude, Status: core.RunFailed}) // no session
	m.reload()
	m.recentRuns, _ = st.ListRecentRuns(50)
	m.view = viewRuns

	// Both the 't' key and its 'enter' alias hit the same path and explain why.
	for _, key := range []string{"t", "enter"} {
		_, cmd := m.Update(keyPress(key))
		if cmd == nil {
			t.Fatalf("%q produced no command", key)
		}
		em, ok := cmd().(errMsg)
		if !ok || !strings.Contains(em.err.Error(), "no session") {
			t.Errorf("%q: got %#v, want an errMsg about no session", key, cmd())
		}
	}
}

func TestRunsResumeSpawnsAndToasts(t *testing.T) {
	if _, err := exec.LookPath("true"); err != nil {
		t.Skip("`true` not available to stand in for a terminal")
	}
	fakeTerminals(t, "linux", "true", "true") // $TERMINAL=true spawns and exits 0
	m, _ := resumableSetup(t)
	_, cmd := m.Update(keyPress("t"))
	if cmd == nil {
		t.Fatal("resume produced no command")
	}
	// On success the action returns a toast, not an error.
	if em, ok := cmd().(errMsg); ok {
		t.Fatalf("resume should succeed, got error: %v", em.err)
	}
}

func TestRunsResumeCopiesWhenNoTerminal(t *testing.T) {
	fakeEnv(t, "linux", map[string]string{}) // no multiplexer, no $TERMINAL, nothing on PATH
	m, _ := resumableSetup(t)
	_, cmd := m.Update(keyPress("t"))
	if cmd == nil {
		t.Fatal("with no terminal, resume should copy the command, not do nothing")
	}
	// It must NOT run inline and must NOT error — it copies the command instead
	// (a clipboard write batched with a toast, neither of which is an errMsg).
	if _, ok := cmd().(errMsg); ok {
		t.Fatal("no terminal should copy the command, not surface an error")
	}
}

func TestRunsWatchTailsLog(t *testing.T) {
	if _, err := exec.LookPath("true"); err != nil {
		t.Skip("`true` not available to stand in for a terminal")
	}
	fakeTerminals(t, "linux", "true", "true")
	m, st, _ := newModelWithManager(t)
	p, _ := st.CreateProject(core.Project{Name: "Jeera", KeyPrefix: "JEE", RepoPath: t.TempDir()})
	iss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "x", Type: core.TypeTask})
	if _, err := st.CreateRun(core.Run{
		IssueID: iss.ID, Version: 1, Provider: core.ProviderClaude,
		Status: core.RunRunning, LogPath: filepath.Join(t.TempDir(), "run.log"),
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	m.reload()
	m.recentRuns, _ = st.ListRecentRuns(50)
	m.view = viewRuns

	_, cmd := m.Update(keyPress("w"))
	if cmd == nil {
		t.Fatal("watch produced no command")
	}
	if em, ok := cmd().(errMsg); ok {
		t.Fatalf("watch should open the log, got error: %v", em.err)
	}
}

func TestRunsWatchNoLogErrors(t *testing.T) {
	m, _ := resumableSetup(t) // the seeded run carries a session id but no log path
	_, cmd := m.Update(keyPress("w"))
	if cmd == nil {
		t.Fatal("w produced no command")
	}
	em, ok := cmd().(errMsg)
	if !ok || !strings.Contains(em.err.Error(), "no log") {
		t.Errorf("watch with no log should explain itself: got %#v", cmd())
	}
}

func TestRunsEmptyState(t *testing.T) {
	m, _ := newTestModel(t)
	m.view = viewRuns
	m.recentRuns = nil
	out := render(m)
	if !strings.Contains(out, "No runs yet") {
		t.Error("an empty Runs view should say how to start one")
	}
	// The footer is contextual: with nothing to resume, it must not advertise it.
	if strings.Contains(out, "resume") {
		t.Error("an empty Runs view should not offer the resume action in the footer")
	}
}

func TestRunsLiveReloadReanchorsCursor(t *testing.T) {
	m, st := newTestModel(t)
	seedRuns(t, &m) // [v2 (newest), v1]
	m.view = viewRuns
	m.runsCursor = 1 // select v1 (the older claude run)
	selID := m.recentRuns[1].ID
	issID := m.recentRuns[0].IssueID

	// A new run starts while the overlay is open — it prepends to the list.
	if _, err := st.CreateRun(core.Run{IssueID: issID, Version: 3, Provider: core.ProviderClaude, Status: core.RunRunning}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	next, _ := m.Update(storeEventMsg{})
	m = next.(Model)

	if m.recentRuns[m.runsCursor].ID != selID {
		t.Errorf("cursor drifted to run %d, want it re-anchored to %d", m.recentRuns[m.runsCursor].ID, selID)
	}
}

func TestRunsClampOnShrink(t *testing.T) {
	m, _ := newTestModel(t)
	seedRuns(t, &m)
	m.view = viewRuns
	m.runsCursor = 5 // out of range
	m.clampRunsCursor()
	if m.runsCursor != 1 {
		t.Errorf("clampRunsCursor = %d, want last index 1", m.runsCursor)
	}
	m.recentRuns = nil
	m.clampRunsCursor()
	if m.runsCursor != 0 {
		t.Errorf("clampRunsCursor on empty = %d, want 0", m.runsCursor)
	}
}
