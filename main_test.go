package main

import "testing"

// TestRunOpensStore exercises the root command's happy path against an isolated
// data directory: it must open (creating + migrating) the store and report
// status without error.
func TestRunOpensStore(t *testing.T) {
	t.Setenv("JEERA_DATA_DIR", t.TempDir())
	for _, mode := range []struct{ headless, noMCP bool }{
		{false, false},
		{true, false},
		{false, true},
	} {
		if err := run(mode.headless, mode.noMCP); err != nil {
			t.Fatalf("run(headless=%v, noMCP=%v): %v", mode.headless, mode.noMCP, err)
		}
	}
}
