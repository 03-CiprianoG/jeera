package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/03-CiprianoG/jeera/internal/mcp"
	"github.com/03-CiprianoG/jeera/internal/paths"
	"github.com/03-CiprianoG/jeera/internal/run"
	"github.com/03-CiprianoG/jeera/internal/store"
)

// Run starts the TUI over the store and an optional running MCP server, blocking
// until the user quits or the context is cancelled. The store's change events
// are bridged into the program so the board refreshes the moment an agent writes
// over MCP — the human's board and the agents never drift apart.
func Run(ctx context.Context, st *store.Store, mcpSrv *mcp.Server) error {
	// The run manager spawns agents and points them at this live MCP endpoint.
	mgr := run.NewManager(st, paths.DataDir(), func() string {
		if mcpSrv == nil {
			return ""
		}
		return mcpSrv.Status().URL
	})
	// Cancel and reap any in-flight runs before the store closes, so no agent
	// process is orphaned and no run writes to a torn-down database.
	defer mgr.Shutdown()

	model := New(st, mcpSrv, mgr)
	p := tea.NewProgram(model)

	// Bridge store change events → the program.
	events, cancel := st.Subscribe()
	defer cancel()
	go func() {
		for ev := range events {
			p.Send(storeEventMsg{ev})
		}
	}()

	// Quit cleanly on SIGINT/SIGTERM (Ctrl-C is also handled as a key).
	go func() {
		<-ctx.Done()
		p.Quit()
	}()

	_, err := p.Run()
	return err
}
