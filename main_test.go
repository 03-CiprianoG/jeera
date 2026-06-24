package main

import (
	"context"
	"io"
	"testing"
)

// TestRunServesAndShutsDown exercises the root command's happy path against an
// isolated data dir and an ephemeral MCP port. A pre-cancelled context makes the
// serve loop return immediately, so each mode opens the store, (optionally)
// starts and gracefully stops the MCP server without error.
func TestRunServesAndShutsDown(t *testing.T) {
	t.Setenv("JEERA_DATA_DIR", t.TempDir())
	t.Setenv("JEERA_MCP_PORT", "0") // bind any free port

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // serve loop exits at once

	for _, mode := range []struct {
		name            string
		headless, noMCP bool
	}{
		{"tui+mcp", false, false},
		{"headless", true, false},
		{"no-mcp", false, true},
	} {
		t.Run(mode.name, func(t *testing.T) {
			if err := run(ctx, mode.headless, mode.noMCP, io.Discard); err != nil {
				t.Fatalf("run: %v", err)
			}
		})
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
