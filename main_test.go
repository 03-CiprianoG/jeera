package main

import (
	"context"
	"io"
	"testing"
)

// TestRunHeadlessServesAndShutsDown exercises the --headless path against an
// isolated data dir and an ephemeral MCP port. A pre-cancelled context makes the
// serve loop return immediately, so it starts and gracefully stops the MCP
// server without error. (Board mode launches a Bubble Tea program that needs a
// TTY, so it is covered by the tui package's tests rather than here.)
func TestRunHeadlessServesAndShutsDown(t *testing.T) {
	t.Setenv("JEERA_DATA_DIR", t.TempDir())
	t.Setenv("JEERA_MCP_PORT", "0") // bind any free port

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // serve loop exits at once

	if err := run(ctx, true /* headless */, false, io.Discard); err != nil {
		t.Fatalf("run(headless): %v", err)
	}
}

func TestMCPPortFromEnv(t *testing.T) {
	t.Setenv("JEERA_MCP_PORT", "9123")
	if got := mcpPort(); got != 9123 {
		t.Errorf("mcpPort() = %d, want 9123", got)
	}
	t.Setenv("JEERA_MCP_PORT", "not-a-port")
	if got := mcpPort(); got == 0 {
		t.Errorf("mcpPort() with bad value = %d, want the default", got)
	}
}
