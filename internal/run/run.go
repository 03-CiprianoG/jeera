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
	store   *store.Store
	dataDir string        // base for run logs, MCP config files and worktrees
	mcpURL  func() string // the live Jeera MCP URL (empty if the server is off)

	mu      sync.Mutex                   // guards cancels
	cancels map[int64]context.CancelFunc // in-flight runs, by run id
	wg      sync.WaitGroup               // tracks launch goroutines
}

// NewManager builds a run manager. mcpURL is a function so the live endpoint is
// read at start time (the server may bind a fallback port).
func NewManager(st *store.Store, dataDir string, mcpURL func() string) *Manager {
	if mcpURL == nil {
		mcpURL = func() string { return "" }
	}
	return &Manager{
		store:   st,
		dataDir: dataDir,
		mcpURL:  mcpURL,
		cancels: make(map[int64]context.CancelFunc),
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

	assignee := issue.Assignee
	if assignee.IsZero() {
		assignee = core.DefaultAssignee(core.ProviderClaude)
	}
	prov := agent.For(assignee.Provider)
	if prov == nil {
		return nil, fmt.Errorf("no driver for provider %q", assignee.Provider)
	}

	permMode := issue.Settings.PermissionMode
	if permMode == "" {
		permMode = "bypassPermissions"
	}
	worktreeOn := true
	if issue.WorktreeOn != nil {
		worktreeOn = *issue.WorktreeOn
	}

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
	// The context lets Stop/Shutdown kill the process; WaitDelay bounds how long
	// a stubborn process tree may keep the pipe open after the kill signal.
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, prov.Binary(), prov.Args(spec)...)
	cmd.Dir = workDir
	cmd.WaitDelay = killGrace
	return &plan{cmd: cmd, ctx: ctx, cancel: cancel, run: run, prov: prov, logPath: logPath}, nil
}

// launch runs the command, streaming its output to the log and updating the run
// as the session id arrives and on completion.
func (m *Manager) launch(pl *plan) {
	defer m.wg.Done()
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
