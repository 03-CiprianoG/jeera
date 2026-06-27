package tui

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/config"
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
	// A temp config keeps tests hermetic — they never read or write the user's real
	// config, so renders that depend on it (the default-project chip) stay
	// deterministic regardless of the machine running them.
	cfg, _ := config.NewStore(filepath.Join(t.TempDir(), "config.toml"))
	m := New(st, nil, nil, nil, cfg)
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

// activateSprint creates an active sprint for the project and sweeps every
// currently-unsprinted issue into it, so a SCRUM board (which shows only the
// active sprint's work) renders them. It returns the sprint id, which the board
// create-flow threads onto new issues. Board tests call it after creating their
// issues and before reload.
func activateSprint(t *testing.T, st *store.Store, projectID int64) int64 {
	t.Helper()
	sp, err := st.CreateSprint(core.Sprint{ProjectID: projectID, Name: "Sprint 1", State: core.SprintActive})
	if err != nil {
		t.Fatalf("CreateSprint: %v", err)
	}
	issues, err := st.ListIssues(store.IssueFilter{ProjectID: projectID, Unsprinted: true})
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	for _, iss := range issues {
		if err := st.AddIssueToSprint(iss.ID, &sp.ID); err != nil {
			t.Fatalf("AddIssueToSprint: %v", err)
		}
	}
	return sp.ID
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
	case "/":
		return tea.KeyPressMsg{Code: '/', Text: "/"}
	case "super+f": // ⌘F
		return tea.KeyPressMsg{Code: 'f', Mod: tea.ModSuper}
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

// captureOutput runs fn with os.Stdout redirected to a pipe and returns whatever
// fn wrote — used to assert on the raw OSC sequences the title commands emit.
func captureOutput(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()
	_ = w.Close()

	var sb strings.Builder
	if _, err := io.Copy(&sb, r); err != nil {
		t.Fatalf("read captured output: %v", err)
	}
	return sb.String()
}

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
