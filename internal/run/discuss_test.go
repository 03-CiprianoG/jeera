package run

import (
	"strings"
	"testing"
)

func TestDiscussCommandArgs(t *testing.T) {
	m, _, iss := setup(t) // setup's mcpURL returns a non-empty URL
	cmd, err := m.DiscussCommand(iss)
	if err != nil {
		t.Fatalf("DiscussCommand: %v", err)
	}
	if cmd.Args[0] != "claude" {
		t.Errorf("discuss should use claude, got %q", cmd.Args[0])
	}
	joined := strings.Join(cmd.Args, " ")
	for _, want := range []string{"--mcp-config", "--strict-mcp-config", iss.Key} {
		if !strings.Contains(joined, want) {
			t.Errorf("discuss args missing %q in:\n%s", want, joined)
		}
	}
	// It must NOT be a -p (headless) run — Discuss is interactive.
	if strings.Contains(joined, "-p ") {
		t.Errorf("discuss should be interactive, not -p: %s", joined)
	}
	project, _ := m.store.GetProject(iss.ProjectID)
	if cmd.Dir != project.RepoPath {
		t.Errorf("discuss should run in the repo, dir=%q want %q", cmd.Dir, project.RepoPath)
	}
}

func TestDiscussCommandNoMCP(t *testing.T) {
	_, st, iss := setup(t)
	// A manager with no live MCP endpoint cannot discuss — the agent would have
	// nothing to load the ticket from.
	m := NewManager(st, t.TempDir(), func() string { return "" }, nil)
	if _, err := m.DiscussCommand(iss); err == nil {
		t.Error("DiscussCommand should error when the MCP server is off")
	}
}
