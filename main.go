// Command jeera is an agentic-first, terminal-native issue tracker. Running it
// starts the human's board (a Bubble Tea TUI) and an embedded MCP server over
// local HTTP in a single process, both backed by one shared local store.
//
// The front-ends are landing incrementally; today this entry point parses the
// final command surface (the root command plus the --headless, --no-mcp and
// version forms), opens the system-of-record store, and serves the embedded MCP
// server so agents can already read and write issues.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/03-CiprianoG/jeera/internal/mcp"
	"github.com/03-CiprianoG/jeera/internal/paths"
	runpkg "github.com/03-CiprianoG/jeera/internal/run"
	"github.com/03-CiprianoG/jeera/internal/schedule"
	"github.com/03-CiprianoG/jeera/internal/store"
	"github.com/03-CiprianoG/jeera/internal/tui"
	"github.com/03-CiprianoG/jeera/internal/version"
)

func main() {
	// `jeera version` is a bare subcommand, matching the documented surface.
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version.String())
		return
	}

	var (
		headless    bool
		noMCP       bool
		showVersion bool
	)
	fs := flag.NewFlagSet("jeera", flag.ExitOnError)
	fs.BoolVar(&headless, "headless", false, "run the MCP server only (no TUI)")
	fs.BoolVar(&noMCP, "no-mcp", false, "run the TUI only (no MCP server)")
	fs.BoolVar(&showVersion, "version", false, "print version and exit")
	fs.Usage = usage(fs)
	_ = fs.Parse(os.Args[1:])

	if showVersion {
		fmt.Println(version.String())
		return
	}

	// Serve until interrupted; the context is cancelled on Ctrl-C / SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, headless, noMCP, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "jeera:", err)
		os.Exit(1)
	}
}

// run opens the shared store and dispatches to the requested mode: the board
// (with an embedded MCP server unless --no-mcp), or MCP-only with --headless.
func run(ctx context.Context, headless, noMCP bool, out io.Writer) error {
	st, err := store.Open(paths.DBPath())
	if err != nil {
		return err
	}
	defer st.Close()

	if headless {
		return runHeadless(ctx, st, out)
	}

	// Board mode: start the MCP server alongside the TUI unless asked not to.
	var srv *mcp.Server
	if !noMCP {
		srv = mcp.NewServer(mcp.NewService(st))
		if err := srv.Start("127.0.0.1", mcpPort()); err != nil {
			return fmt.Errorf("start MCP server: %w", err)
		}
		defer func() {
			shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutCtx)
		}()
	}
	return tui.Run(ctx, st, srv)
}

// runHeadless serves only the MCP server, printing connection details and
// blocking until the context is cancelled.
func runHeadless(ctx context.Context, st *store.Store, out io.Writer) error {
	srv := mcp.NewServer(mcp.NewService(st))
	if err := srv.Start("127.0.0.1", mcpPort()); err != nil {
		return fmt.Errorf("start MCP server: %w", err)
	}
	status := srv.Status()
	projects, _ := st.ListProjects()

	// The execution engine and scheduler run headless too, so "Schedule Start"
	// entries fire while the server is up — a quiet machine working its backlog.
	mgr := runpkg.NewManager(st, paths.DataDir(), func() string { return srv.Status().URL })
	defer mgr.Shutdown()
	scheduled := 0
	if sched, err := schedule.New(st, mgr); err == nil {
		if err := sched.Start(); err == nil {
			defer sched.Shutdown()
			if active, _ := st.ListEnabledSchedules(); active != nil {
				scheduled = len(active)
			}
		}
	}

	fmt.Fprintln(out, version.String())
	fmt.Fprintf(out, "store:     %s\n", paths.DBPath())
	fmt.Fprintf(out, "projects:  %d\n", len(projects))
	fmt.Fprintf(out, "schedules: %d enabled\n", scheduled)
	fmt.Fprintln(out, "mode:      MCP server only (--headless)")
	fmt.Fprintf(out, "mcp:       %s\n", status.URL)
	fmt.Fprintln(out, "\nConnect an agent:")
	fmt.Fprintf(out, "  claude mcp add --transport http jeera %s\n", status.URL)
	fmt.Fprintln(out, "\nor add to .mcp.json:")
	fmt.Fprintln(out, srv.ClientConfigJSON())
	fmt.Fprintln(out, "\nServing MCP until interrupted (Ctrl-C).")

	<-ctx.Done()
	fmt.Fprintln(out, "\nshutting down…")
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutCtx)
}

// mcpPort resolves the preferred MCP port: JEERA_MCP_PORT if set and valid,
// otherwise the package default. The server falls back to a nearby free port if
// this one is taken.
func mcpPort() int {
	if v := os.Getenv("JEERA_MCP_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p >= 0 && p <= 65535 {
			return p
		}
	}
	return mcp.DefaultPort
}

func usage(fs *flag.FlagSet) func() {
	return func() {
		fmt.Fprintln(fs.Output(), "Jeera — agentic-first, terminal-native issue tracker.")
		fmt.Fprintln(fs.Output(), "\nUsage:")
		fmt.Fprintln(fs.Output(), "  jeera            start the TUI and the embedded MCP server")
		fmt.Fprintln(fs.Output(), "  jeera --headless start only the MCP server")
		fmt.Fprintln(fs.Output(), "  jeera --no-mcp   start only the TUI")
		fmt.Fprintln(fs.Output(), "  jeera version    print version and exit")
		fmt.Fprintln(fs.Output(), "\nFlags:")
		fs.PrintDefaults()
	}
}
