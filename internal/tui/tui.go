package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/03-CiprianoG/jeera/internal/mcp"
	"github.com/03-CiprianoG/jeera/internal/store"
)

// Run starts the TUI over the store and an optional running MCP server, blocking
// until the user quits or the context is cancelled. The store's change events
// are bridged into the program so the board refreshes the moment an agent writes
// over MCP — the human's board and the agents never drift apart.
func Run(ctx context.Context, st *store.Store, mcpSrv *mcp.Server) error {
	model := New(st, mcpSrv)
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
