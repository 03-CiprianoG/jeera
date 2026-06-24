package run

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/03-CiprianoG/jeera/internal/agent"
	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/store"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "T"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644)
	for _, args := range [][]string{{"add", "."}, {"commit", "-q", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.CombinedOutput()
	}
	return dir
}

func setup(t *testing.T) (*Manager, *store.Store, core.Issue) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "jeera.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	repo := gitRepo(t)
	p, _ := st.CreateProject(core.Project{Name: "Jeera", KeyPrefix: "JEE", RepoPath: repo})
	iss, _ := st.CreateIssue(core.Issue{
		ProjectID: p.ID, Title: "Build it", Type: core.TypeStory,
		Assignee: core.Assignee{Provider: core.ProviderClaude, Model: "opus", Effort: core.EffortHigh},
	})
	m := NewManager(st, t.TempDir(), func() string { return "http://127.0.0.1:7777" }, nil)
	return m, st, iss
}

func TestPrepareWithWorktree(t *testing.T) {
	m, st, iss := setup(t)
	pl, err := m.prepare(iss)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	defer pl.cancel()
	run := pl.run
	if run.Status != core.RunRunning || run.Version != 1 {
		t.Errorf("run = %+v, want running v1", run)
	}
	if pl.prov.Name() != core.ProviderClaude {
		t.Errorf("provider = %v", pl.prov.Name())
	}
	if run.SessionID == "" {
		t.Error("claude run should pre-assign a session id")
	}
	if run.WorktreePath == "" {
		t.Fatal("worktree path should be set")
	}
	if st, err := os.Stat(run.WorktreePath); err != nil || !st.IsDir() {
		t.Errorf("worktree dir should exist: %v", err)
	}
	if pl.cmd.Dir != run.WorktreePath {
		t.Errorf("cmd.Dir = %q, want worktree %q", pl.cmd.Dir, run.WorktreePath)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(pl.logPath), "mcp.json")); err != nil {
		t.Errorf("mcp config should be written: %v", err)
	}
	args := strings.Join(pl.cmd.Args, " ")
	if pl.cmd.Args[0] != "claude" {
		t.Errorf("binary = %q, want claude", pl.cmd.Args[0])
	}
	for _, want := range []string{"--mcp-config", "--strict-mcp-config", "--session-id " + run.SessionID, "--permission-mode bypassPermissions"} {
		if !strings.Contains(args, want) {
			t.Errorf("cmd args missing %q in:\n%s", want, args)
		}
	}
	// The run row and its log path are persisted.
	got, err := st.GetRun(run.ID)
	if err != nil || got.LogPath != pl.logPath {
		t.Errorf("run not persisted with log path: %+v err=%v", got, err)
	}
}

func TestPrepareWithoutWorktree(t *testing.T) {
	m, _, iss := setup(t)
	off := false
	iss.WorktreeOn = &off
	pl, err := m.prepare(iss)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	defer pl.cancel()
	if pl.run.WorktreePath != "" {
		t.Errorf("worktree should be off, got %q", pl.run.WorktreePath)
	}
	project, _ := m.store.GetProject(iss.ProjectID)
	if pl.cmd.Dir != project.RepoPath {
		t.Errorf("cmd.Dir = %q, want repo %q", pl.cmd.Dir, project.RepoPath)
	}
}

func TestPrepareDefaultsAssignee(t *testing.T) {
	m, st, _ := setup(t)
	// An issue with no assignee defaults to claude.
	p, _ := st.GetProjectByPrefix("JEE")
	iss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "unassigned"})
	pl, err := m.prepare(iss)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	defer pl.cancel()
	if pl.prov.Name() != core.ProviderClaude || pl.run.Model == "" {
		t.Errorf("unassigned issue should default to a claude model, got %v/%q", pl.prov.Name(), pl.run.Model)
	}
}

func TestPrepareCodex(t *testing.T) {
	m, st, _ := setup(t)
	p, _ := st.GetProjectByPrefix("JEE")
	iss, _ := st.CreateIssue(core.Issue{
		ProjectID: p.ID, Title: "codex work",
		Assignee: core.Assignee{Provider: core.ProviderCodex, Model: "gpt-5.4", Effort: core.EffortMedium},
	})
	pl, err := m.prepare(iss)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	defer pl.cancel()
	// Codex mints its own thread id — no pre-assigned session.
	if pl.run.SessionID != "" {
		t.Errorf("codex run should not pre-assign a session id, got %q", pl.run.SessionID)
	}
	if pl.cmd.Args[0] != "codex" {
		t.Errorf("binary = %q, want codex", pl.cmd.Args[0])
	}
	args := strings.Join(pl.cmd.Args, " ")
	for _, want := range []string{"exec --json", "-m gpt-5.4", "mcp_servers.jeera.url="} {
		if !strings.Contains(args, want) {
			t.Errorf("codex args missing %q in:\n%s", want, args)
		}
	}
}

func TestPrepareUnknownProvider(t *testing.T) {
	m, st, _ := setup(t)
	p, _ := st.GetProjectByPrefix("JEE")
	// An unknown provider can reach the engine even though the store would reject
	// it on write (e.g. via a future import path), so prepare must guard it. A
	// non-empty model keeps the assignee non-zero so it is honored, not defaulted.
	iss := core.Issue{
		ProjectID: p.ID, Title: "who runs this",
		Assignee: core.Assignee{Provider: core.Provider("gemini"), Model: "x"},
	}
	_, err := m.prepare(iss)
	if err == nil || !strings.Contains(err.Error(), "no driver for provider") {
		t.Errorf("expected a no-driver error, got %v", err)
	}
}

func TestPrepareWorktreeAddFails(t *testing.T) {
	m, _, iss := setup(t)
	// Pre-occupy the worktree path so git worktree add fails, proving the error
	// is wrapped rather than swallowed.
	clash := filepath.Join(m.dataDir, "worktrees", fmt.Sprintf("%s-v1", iss.Key))
	if err := os.MkdirAll(clash, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(clash, "occupied"), []byte("x"), 0o644)
	_, err := m.prepare(iss)
	if err == nil || !strings.Contains(err.Error(), "create worktree") {
		t.Errorf("expected a create-worktree error, got %v", err)
	}
}

func TestPrepareNoMCP(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "jeera.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	repo := gitRepo(t)
	p, _ := st.CreateProject(core.Project{Name: "Jeera", KeyPrefix: "JEE", RepoPath: repo})
	iss, _ := st.CreateIssue(core.Issue{
		ProjectID: p.ID, Title: "no mcp",
		Assignee: core.Assignee{Provider: core.ProviderClaude, Model: "opus"},
	})
	// MCP server off: prepare must write no mcp.json and omit --mcp-config.
	m := NewManager(st, t.TempDir(), func() string { return "" }, nil)
	pl, err := m.prepare(iss)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	defer pl.cancel()
	if _, err := os.Stat(filepath.Join(filepath.Dir(pl.logPath), "mcp.json")); !os.IsNotExist(err) {
		t.Errorf("mcp.json should not be written when MCP is off (err=%v)", err)
	}
	if strings.Contains(strings.Join(pl.cmd.Args, " "), "--mcp-config") {
		t.Errorf("claude args should omit --mcp-config when MCP is off: %v", pl.cmd.Args)
	}
}

// --- launch lifecycle, exercised with a fake provider CLI ----------------------

// fakePlan builds a plan whose command re-execs this test binary as a canned
// provider CLI (TestHelperProcess), so the full launch path — streaming, session
// capture, exit handling — runs in CI without a real agent.
func fakePlan(t *testing.T, m *Manager, run core.Run, stdout string, exit, sleepMS int) *plan {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = append(os.Environ(),
		"GO_WANT_HELPER_PROCESS=1",
		"GO_HELPER_STDOUT="+stdout,
		"GO_HELPER_EXIT="+strconv.Itoa(exit),
		"GO_HELPER_SLEEP_MS="+strconv.Itoa(sleepMS),
	)
	cmd.WaitDelay = killGrace
	return &plan{
		cmd: cmd, ctx: ctx, cancel: cancel, run: run,
		prov: agent.For(core.ProviderClaude), logPath: filepath.Join(t.TempDir(), "run.log"),
	}
}

func newRunRow(t *testing.T, st *store.Store, iss core.Issue) core.Run {
	t.Helper()
	r, err := st.CreateRun(core.Run{IssueID: iss.ID, Version: 1, Provider: core.ProviderClaude, Status: core.RunRunning})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	return r
}

func TestLaunchSuccessCapturesSessionAndLog(t *testing.T) {
	m, st, iss := setup(t)
	run := newRunRow(t, st, iss)
	stdout := `{"type":"system","subtype":"init","session_id":"sess-xyz"}` + "\n" +
		`{"type":"result","result":"done","is_error":false}`
	pl := fakePlan(t, m, run, stdout, 0, 0)

	m.wg.Add(1)
	m.launch(pl)

	got, _ := st.GetRun(run.ID)
	if got.Status != core.RunSucceeded {
		t.Errorf("status = %v, want succeeded", got.Status)
	}
	if got.SessionID != "sess-xyz" {
		t.Errorf("session id = %q, want captured from stream", got.SessionID)
	}
	if got.ExitCode == nil || *got.ExitCode != 0 {
		t.Errorf("exit code = %v, want 0", got.ExitCode)
	}
	if got.EndedAt == nil {
		t.Error("ended_at should be set")
	}
	data, _ := os.ReadFile(pl.logPath)
	if !strings.Contains(string(data), "sess-xyz") {
		t.Errorf("log should contain streamed output, got:\n%s", data)
	}
}

func TestLaunchRecordsFailureExitCode(t *testing.T) {
	m, st, iss := setup(t)
	run := newRunRow(t, st, iss)
	pl := fakePlan(t, m, run, "", 3, 0)

	m.wg.Add(1)
	m.launch(pl)

	got, _ := st.GetRun(run.ID)
	if got.Status != core.RunFailed {
		t.Errorf("status = %v, want failed", got.Status)
	}
	if got.ExitCode == nil || *got.ExitCode != 3 {
		t.Errorf("exit code = %v, want 3", got.ExitCode)
	}
}

func TestLaunchStartFailureLogsError(t *testing.T) {
	m, st, iss := setup(t)
	run := newRunRow(t, st, iss)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, filepath.Join(t.TempDir(), "definitely-not-a-binary"))
	pl := &plan{
		cmd: cmd, ctx: ctx, cancel: cancel, run: run,
		prov: agent.For(core.ProviderClaude), logPath: filepath.Join(t.TempDir(), "run.log"),
	}

	m.wg.Add(1)
	m.launch(pl)

	got, _ := st.GetRun(run.ID)
	if got.Status != core.RunFailed {
		t.Errorf("status = %v, want failed", got.Status)
	}
	if got.ExitCode == nil || *got.ExitCode != startExitCode {
		t.Errorf("exit code = %v, want sentinel %d for a spawn failure", got.ExitCode, startExitCode)
	}
	data, _ := os.ReadFile(pl.logPath)
	if !strings.Contains(string(data), "start") {
		t.Errorf("spawn failure should be logged, got:\n%s", data)
	}
}

func TestShutdownCancelsInFlightRun(t *testing.T) {
	m, st, iss := setup(t)
	run := newRunRow(t, st, iss)
	// A process that announces its session immediately, then sleeps long enough
	// that Shutdown must cancel and reap it.
	pl := fakePlan(t, m, run, `{"type":"system","subtype":"init","session_id":"live-1"}`, 0, 30000)

	m.mu.Lock()
	m.cancels[run.ID] = pl.cancel
	m.mu.Unlock()
	m.wg.Add(1)
	go m.launch(pl)

	// Wait until the run is genuinely live (session captured from the stream)
	// before cancelling, so we exercise the kill-while-running path.
	live := false
	for i := 0; i < 100; i++ {
		if got, _ := st.GetRun(run.ID); got.SessionID == "live-1" {
			live = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !live {
		t.Fatal("run never started streaming")
	}

	done := make(chan struct{})
	go func() { m.Shutdown(); close(done) }()
	select {
	case <-done:
	case <-time.After(killGrace + 5*time.Second):
		t.Fatal("Shutdown did not return — the run was not reaped")
	}

	got, _ := st.GetRun(run.ID)
	if got.Status != core.RunCancelled {
		t.Errorf("status = %v, want cancelled", got.Status)
	}
}

// TestHelperProcess is not a real test — it is re-execed by fakePlan to stand in
// for a provider CLI, emitting canned stdout then exiting with a chosen code.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	if out := os.Getenv("GO_HELPER_STDOUT"); out != "" {
		for _, line := range strings.Split(out, "\n") {
			fmt.Fprintln(os.Stdout, line)
		}
	}
	if ms, _ := strconv.Atoi(os.Getenv("GO_HELPER_SLEEP_MS")); ms > 0 {
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
	code, _ := strconv.Atoi(os.Getenv("GO_HELPER_EXIT"))
	os.Exit(code)
}
