package tui

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/store"
)

// lineWidth is the display-cell width of an (ANSI-stripped) line.
func lineWidth(s string) int { return lipgloss.Width(s) }

var updateGolden = flag.Bool("update", false, "update golden files")

// newTestModel builds a model over a fresh store, sized to a fixed 100x30 so
// renders are deterministic.
func newTestModel(t *testing.T) (Model, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "jeera.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	m := New(st, nil, nil, nil, nil)
	m.width, m.height = 100, 30
	return m, st
}

func seedProject(t *testing.T, st *store.Store) core.Project {
	t.Helper()
	p, err := st.CreateProject(core.Project{Name: "Jeera", KeyPrefix: "JEE", RepoPath: "/tmp/jeera"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return p
}

// keyPress builds a KeyPressMsg for a single character or a named special key.
func keyPress(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "alt+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModAlt}
	case "alt+shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModAlt | tea.ModShift}
	case "shift+left":
		return tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift}
	case "shift+right":
		return tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	}
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// render returns the model's current view content with ANSI styling stripped,
// so golden comparisons assert on layout and text rather than environment-
// dependent color codes.
func render(m Model) string {
	return strings.TrimRight(stripANSI(m.View().Content), "\n ")
}

func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

// goldenFile compares got against testdata/<name>.golden (or rewrites it with
// -update).
func goldenFile(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run: go test ./internal/tui -update): %v", path, err)
	}
	if got != string(want) {
		t.Errorf("golden mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got, string(want))
	}
}
