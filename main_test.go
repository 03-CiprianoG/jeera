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

func TestMCPPortResolution(t *testing.T) {
	t.Setenv("JEERA_MCP_PORT", "9123")
	if got := mcpPort(0); got != 9123 {
		t.Errorf("env should win: mcpPort(0) = %d, want 9123", got)
	}
	// The env var also wins over a configured port.
	if got := mcpPort(8000); got != 9123 {
		t.Errorf("env should win over config: mcpPort(8000) = %d, want 9123", got)
	}

	t.Setenv("JEERA_MCP_PORT", "")
	if got := mcpPort(8000); got != 8000 {
		t.Errorf("configured port should be used when no env: mcpPort(8000) = %d, want 8000", got)
	}
	if got := mcpPort(0); got == 0 {
		t.Errorf("with no env and no config, mcpPort(0) = %d, want the default", got)
	}

	t.Setenv("JEERA_MCP_PORT", "not-a-port")
	if got := mcpPort(0); got == 0 {
		t.Errorf("a bad env value should fall back, got %d", got)
	}
}
