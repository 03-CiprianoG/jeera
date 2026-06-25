package tui

import (
	"os"
	"testing"

	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

func TestZZDumpHeader(t *testing.T) {
	if os.Getenv("DUMP") == "" {
		t.Skip("set DUMP=path to write the header")
	}
	th := theme.New()
	items := []navItem{{iconBoard, "Board"}, {iconBacklog, "Backlog"}, {iconSprints, "Sprints"}, {iconRuns, "Runs"}}
	out := navbar(th, 90, items, 0, brandLogo(th))
	if err := os.WriteFile(os.Getenv("DUMP"), []byte(out+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
