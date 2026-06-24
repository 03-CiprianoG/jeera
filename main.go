// Command jeera is an agentic-first, terminal-native issue tracker. Running it
// starts the human's board (a Bubble Tea TUI) and an embedded MCP server over
// local HTTP in a single process, both backed by one shared local store.
//
// During early development the front-ends are landing incrementally; this entry
// point already parses the final command surface (the root command plus the
// --headless, --no-mcp and version forms) and opens the system-of-record store.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/03-CiprianoG/jeera/internal/paths"
	"github.com/03-CiprianoG/jeera/internal/store"
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

	if err := run(headless, noMCP); err != nil {
		fmt.Fprintln(os.Stderr, "jeera:", err)
		os.Exit(1)
	}
}

// run opens the shared store and reports status. The TUI and MCP server are
// wired in over the next releases; until then this verifies the store and data
// directory resolve correctly on the host.
func run(headless, noMCP bool) error {
	dbPath := paths.DBPath()
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	projects, err := st.ListProjects()
	if err != nil {
		return err
	}

	mode := "TUI + MCP server"
	switch {
	case headless:
		mode = "MCP server only (--headless)"
	case noMCP:
		mode = "TUI only (--no-mcp)"
	}

	fmt.Println(version.String())
	fmt.Printf("store:  %s\n", dbPath)
	fmt.Printf("mode:   %s\n", mode)
	fmt.Printf("projects: %d\n", len(projects))
	fmt.Println("\nThe interactive board and the embedded MCP server arrive in the next release.")
	return nil
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
