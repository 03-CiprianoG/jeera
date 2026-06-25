// Package run is Jeera's execution engine. It turns "Start a ticket" into a real
// agent process: it resolves the ticket's run settings, optionally isolates the
// work in a git worktree, points the agent at Jeera's own MCP server, spawns the
// provider CLI, and tracks the run — streaming output to a log and recording the
// session id and final status. The agent itself moves the ticket through its
// statuses via MCP, so the board reflects progress live.
package run

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/03-CiprianoG/jeera/internal/agent"
	"github.com/03-CiprianoG/jeera/internal/config"
	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/store"
	"github.com/03-CiprianoG/jeera/internal/worktree"
)

// startExitCode marks a run that never produced a normal process exit — the
// provider binary could not be spawned, or the run was cancelled. It is distinct
// from any real non-zero exit so the failure mode is legible in the run row.
const startExitCode = -1

// killGrace bounds how long a cancelled run's process tree may hold the output
// pipe open after the kill signal before Wait gives up, so Shutdown cannot hang.
const killGrace = 10 * time.Second

// Manager starts and tracks runs over the shared store.
type Manager struct {
	store    *store.Store
	dataDir  string                 // base for run logs, MCP config files and worktrees
	mcpURL   func() string          // the live Jeera MCP URL (empty if the server is off)
	defaults func() config.Defaults // the live global defaults for the settings cascade

	ctx  context.Context    // lifecycle; cancelled by Shutdown
	stop context.CancelFunc // cancels ctx

	mu      sync.Mutex                   // guards cancels
	cancels map[int64]context.CancelFunc // in-flight runs, by run id
	wg      sync.WaitGroup               // tracks launch goroutines
}

// NewManager builds a run manager. mcpURL and defaults are functions so the live
// values are read at start time (the server may bind a fallback port; the user
// may edit the global defaults). Either may be nil, in which case sensible
// built-ins are used.
func NewManager(st *store.Store, dataDir string, mcpURL func() string, defaults func() config.Defaults) *Manager {
	if mcpURL == nil {
		mcpURL = func() string { return "" }
	}
	if defaults == nil {
		defaults = func() config.Defaults { return config.Default().Defaults }
	}
	ctx, stop := context.WithCancel(context.Background())
	return &Manager{
		store:    st,
		dataDir:  dataDir,
		mcpURL:   mcpURL,
		defaults: defaults,
		ctx:      ctx,
		stop:     stop,
		cancels:  make(map[int64]context.CancelFunc),
	}
}

// plan is everything prepare() resolves for a run, up to (but not including)
// spawning the process. It is built separately so the orchestration is testable
// without running a real agent.
type plan struct {
	cmd     *exec.Cmd
	ctx     context.Context
	cancel  context.CancelFunc
	run     core.Run
	prov    agent.Provider
	logPath string
}

// Start launches a run for an issue and returns the created run (status
// running). The process runs in the background; the run row is updated as the
// agent emits its session id and as it completes. The run is tracked so it can
// be cancelled and reaped by Stop/Shutdown.
func (m *Manager) Start(issue core.Issue) (core.Run, error) {
	pl, err := m.prepare(issue)
	if err != nil {
		return core.Run{}, err
	}
	m.mu.Lock()
	m.cancels[pl.run.ID] = pl.cancel
	m.mu.Unlock()
	m.wg.Add(1)
	go m.launch(pl)
	return pl.run, nil
}

// DiscussCommand builds the interactive "Expand / Discuss" command: an
// interactive claude session with Jeera's MCP attached and a preloaded prompt to
// open the ticket for a conversation rather than autonomous work. It is the raw
// provider command; the TUI hosts it in a new terminal window (never inline). It
// writes a small MCP config under the data dir and errors if no MCP endpoint is
// live (the agent would have nothing to load the ticket from).
func (m *Manager) DiscussCommand(issue core.Issue) (*exec.Cmd, error) {
	mcpURL := m.mcpURL()
	if mcpURL == "" {
		return nil, fmt.Errorf("the MCP server is off; run jeera without --no-mcp to discuss a ticket")
	}
	project, err := m.store.GetProject(issue.ProjectID)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(m.dataDir, "discuss")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	cfgPath := filepath.Join(dir, "mcp.json")
	cfg := fmt.Sprintf(`{"mcpServers":{"jeera":{"type":"http","url":%q}}}`, mcpURL)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		return nil, err
	}
	// Interactive claude: no -p, the ticket prompt as the initial message, Jeera's
	// MCP attached so the agent can load and discuss the very ticket.
	cmd := exec.Command("claude",
		"--mcp-config", cfgPath, "--strict-mcp-config",
		agent.DiscussPrompt(issue.Key),
	)
	if project.RepoPath != "" {
		cmd.Dir = project.RepoPath
	}
	return cmd, nil
}

// ResumeCommand builds the command that re-opens a past run's agent session
// interactively, so the user can pick the conversation back up in their own
// terminal (e.g. `claude --resume <id>`). It is the raw provider command — the
// caller (the TUI) wraps it in a terminal emulator to give it a window. It runs
// in the run's worktree while that still exists, else the project repo, and
// errors if the run never captured a session id or its provider is unknown.
func (m *Manager) ResumeCommand(r core.Run) (*exec.Cmd, error) {
	prov := agent.For(r.Provider)
	if prov == nil {
		return nil, fmt.Errorf("no driver for provider %q", r.Provider)
	}
	if r.SessionID == "" {
		return nil, fmt.Errorf("run v%d has no session to resume", r.Version)
	}
	dir, err := m.resumeDir(r)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(prov.Binary(), prov.ResumeArgs(r.SessionID)...)
	cmd.Dir = dir
	return cmd, nil
}

// resumeDir is the working directory a resumed run should open in: its worktree
// while that still exists on disk, otherwise the project repo. A run whose
// worktree was pruned can still be resumed against the repo it branched from.
func (m *Manager) resumeDir(r core.Run) (string, error) {
	if r.WorktreePath != "" {
		if fi, err := os.Stat(r.WorktreePath); err == nil && fi.IsDir() {
			return r.WorktreePath, nil
		}
	}
	issue, err := m.store.GetIssue(r.IssueID)
	if err != nil {
		return "", err
	}
	project, err := m.store.GetProject(issue.ProjectID)
	if err != nil {
		return "", err
	}
	if project.RepoPath == "" {
		return "", fmt.Errorf("project %s has no repo path set", project.KeyPrefix)
	}
	return project.RepoPath, nil
}

// Stop cancels a single in-flight run, killing its process.
func (m *Manager) Stop(runID int64) {
	m.mu.Lock()
	cancel := m.cancels[runID]
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Shutdown cancels every in-flight run and waits for the launch goroutines to
// record their final status. Call it before closing the store so runs neither
// write to a torn-down database nor leave an agent process orphaned.
func (m *Manager) Shutdown() {
	m.stop() // signal sequenced runs to stop spawning, and cancel derived contexts
	m.mu.Lock()
	for _, cancel := range m.cancels {
		cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
}

// prepare resolves settings, sets up the worktree and MCP config, creates the
// run row and builds the command.
func (m *Manager) prepare(issue core.Issue) (*plan, error) {
	project, err := m.store.GetProject(issue.ProjectID)
	if err != nil {
		return nil, err
	}

	// Resolve the run settings through the issue → project → global cascade.
	settings := config.ResolveRun(m.defaults(), project, issue)
	assignee := settings.Assignee
	prov := agent.For(assignee.Provider)
	if prov == nil {
		return nil, fmt.Errorf("no driver for provider %q", assignee.Provider)
	}
	permMode := settings.PermissionMode
	worktreeOn := settings.WorktreeOn

	version, err := m.store.NextRunVersion(issue.ID)
	if err != nil {
		return nil, err
	}

	// Working directory: the project repo, or an isolated worktree of it.
	repo := project.RepoPath
	workDir, branch, wtPath := repo, "", ""
	if worktreeOn && repo != "" && worktree.IsRepo(repo) {
		branch = worktree.SanitizeBranch(fmt.Sprintf("jeera/%s-v%d", strings.ToLower(issue.Key), version))
		wtPath = filepath.Join(m.dataDir, "worktrees", fmt.Sprintf("%s-v%d", issue.Key, version))
		if err := worktree.Add(repo, wtPath, branch, "HEAD"); err != nil {
			return nil, fmt.Errorf("create worktree: %w", err)
		}
		workDir = wtPath
	}

	sessionID := ""
	if prov.PreassignsSession() {
		sessionID = uuid.NewString()
	}

	now := time.Now().UTC()
	run := core.Run{
		IssueID: issue.ID, Version: version, Provider: assignee.Provider,
		Model: assignee.Model, Effort: assignee.Effort, SessionID: sessionID,
		WorktreePath: wtPath, Branch: branch, Status: core.RunRunning,
		PermissionMode: permMode, StartedAt: &now,
	}
	run, err = m.store.CreateRun(run)
	if err != nil {
		return nil, err
	}

	// Per-run artifacts live under dataDir/runs/<id>.
	runDir := filepath.Join(m.dataDir, "runs", fmt.Sprintf("%d", run.ID))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return nil, err
	}
	logPath := filepath.Join(runDir, "run.log")
	run.LogPath = logPath

	mcpConfigPath, mcpURL := "", m.mcpURL()
	if mcpURL != "" {
		mcpConfigPath = filepath.Join(runDir, "mcp.json")
		cfg := fmt.Sprintf(`{"mcpServers":{"jeera":{"type":"http","url":%q}}}`, mcpURL)
		if err := os.WriteFile(mcpConfigPath, []byte(cfg), 0o644); err != nil {
			return nil, err
		}
	}
	if err := m.store.UpdateRun(run); err != nil { // persist the log path
		return nil, err
	}

	spec := agent.RunSpec{
		Prompt:         agent.RunPrompt(issue.Key, issue.Title),
		Model:          assignee.Model,
		Effort:         assignee.Effort,
		SessionID:      sessionID,
		WorkDir:        workDir,
		MCPConfigPath:  mcpConfigPath,
		MCPURL:         mcpURL,
		PermissionMode: permMode,
	}
	// The context lets Stop/Shutdown kill the process; deriving it from the
	// manager's lifecycle context means Shutdown cancels in-flight runs too.
	// WaitDelay bounds how long a stubborn process tree may keep the pipe open
	// after the kill signal.
	ctx, cancel := context.WithCancel(m.ctx)
	cmd := exec.CommandContext(ctx, prov.Binary(), prov.Args(spec)...)
	cmd.Dir = workDir
	cmd.WaitDelay = killGrace
	return &plan{cmd: cmd, ctx: ctx, cancel: cancel, run: run, prov: prov, logPath: logPath}, nil
}

// launch is the goroutine body for a single background Start: run the plan, then
// release the wait-group slot.
func (m *Manager) launch(pl *plan) {
	defer m.wg.Done()
	m.execute(pl)
}

// execute runs a prepared plan to completion (blocking), streaming its output to
// the log and updating the run as the session id arrives and when it finishes. It
// owns the per-run cancel cleanup.
func (m *Manager) execute(pl *plan) {
	defer func() {
		m.mu.Lock()
		delete(m.cancels, pl.run.ID)
		m.mu.Unlock()
		pl.cancel() // release the context whatever the exit path
	}()

	run := pl.run
	logf, _ := os.Create(pl.logPath)
	if logf != nil {
		defer logf.Close()
		pl.cmd.Stderr = logf
	}

	stdout, err := pl.cmd.StdoutPipe()
	if err != nil {
		m.fail(logf, run, "open stdout pipe", err)
		return
	}
	if err := pl.cmd.Start(); err != nil {
		// A cancel that lands before the process starts is a cancellation, not a
		// spawn failure.
		if pl.ctx.Err() != nil {
			code := startExitCode
			m.finish(logf, run, core.RunCancelled, &code)
			return
		}
		m.fail(logf, run, "start "+pl.cmd.Path, err)
		return
	}

	// bufio.Reader (not Scanner) so an arbitrarily large stream-json line — a big
	// tool result or file dump — never aborts the parse loop and stalls the pipe.
	br := bufio.NewReader(stdout)
	for {
		line, readErr := br.ReadBytes('\n')
		if len(line) > 0 {
			if logf != nil {
				logf.Write(line)
			}
			if ev, ok := pl.prov.ParseLine(bytes.TrimRight(line, "\r\n")); ok {
				if ev.Kind == agent.EventSessionStarted && run.SessionID == "" && ev.SessionID != "" {
					run.SessionID = ev.SessionID
					m.persist(logf, run)
				}
			}
		}
		if readErr != nil {
			if readErr != io.EOF && logf != nil {
				fmt.Fprintf(logf, "\n[jeera] read error: %v\n", readErr)
			}
			break
		}
	}

	waitErr := pl.cmd.Wait()
	status, code := core.RunSucceeded, 0
	switch {
	case pl.ctx.Err() != nil:
		// The run was cancelled (Stop/Shutdown); the kill surfaces as a Wait error.
		status, code = core.RunCancelled, startExitCode
	case waitErr != nil:
		status = core.RunFailed
		if ee, ok := waitErr.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = startExitCode
		}
	}
	m.finish(logf, run, status, &code)
}

// fail records a run that could not be spawned, logging the cause so the empty
// run is debuggable rather than silent.
func (m *Manager) fail(logf *os.File, run core.Run, what string, err error) {
	if logf != nil {
		fmt.Fprintf(logf, "[jeera] %s: %v\n", what, err)
	}
	code := startExitCode
	m.finish(logf, run, core.RunFailed, &code)
}

func (m *Manager) finish(logf *os.File, run core.Run, status core.RunStatus, code *int) {
	now := time.Now().UTC()
	run.Status = status
	run.EndedAt = &now
	run.ExitCode = code
	m.persist(logf, run)
}

// persist saves run state, recording any failure to the run log rather than
// swallowing it — e.g. a write after the store was closed on shutdown, which
// would otherwise silently lose the run's final status.
func (m *Manager) persist(logf *os.File, run core.Run) {
	if err := m.store.UpdateRun(run); err != nil && logf != nil {
		fmt.Fprintf(logf, "\n[jeera] failed to persist run state: %v\n", err)
	}
}
