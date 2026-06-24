package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/03-CiprianoG/jeera/internal/config"
	"github.com/03-CiprianoG/jeera/internal/mcp"
	"github.com/03-CiprianoG/jeera/internal/paths"
	"github.com/03-CiprianoG/jeera/internal/run"
	"github.com/03-CiprianoG/jeera/internal/schedule"
	"github.com/03-CiprianoG/jeera/internal/store"
)

// Run starts the TUI over the store and an optional running MCP server, blocking
// until the user quits or the context is cancelled. The store's change events
// are bridged into the program so the board refreshes the moment an agent writes
// over MCP — the human's board and the agents never drift apart.
func Run(ctx context.Context, st *store.Store, mcpSrv *mcp.Server, cfgStore *config.Store) error {
	// The settings cascade backs both the run manager's defaults and the settings
	// view. Guard against a nil store so callers need not always supply one.
	if cfgStore == nil {
		cfgStore, _ = config.NewStore(config.Path())
	}

	// The run manager spawns agents and points them at this live MCP endpoint.
	mgr := run.NewManager(st, paths.DataDir(), func() string {
		if mcpSrv == nil {
			return ""
		}
		return mcpSrv.Status().URL
	}, cfgStore.Defaults)
	// Cancel and reap any in-flight runs before the store closes, so no agent
	// process is orphaned and no run writes to a torn-down database.
	defer mgr.Shutdown()

	// The scheduler fires "Schedule Start" entries while Jeera is up, re-registering
	// the persisted ones on boot. It's best-effort: a scheduler that fails to start
	// just means timed runs are unavailable this session, not that the board can't open.
	sched, err := schedule.New(st, mgr)
	if err == nil {
		if err := sched.Start(); err != nil {
			sched = nil
		} else {
			defer sched.Shutdown()
		}
	} else {
		sched = nil
	}

	model := New(st, mcpSrv, mgr, sched, cfgStore)
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

	_, err = p.Run()
	return err
}
