package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestWindowTitle(t *testing.T) {
	m, st := newTestModel(t)

	// Before any project loads, the title is just the app name.
	if got := m.View().WindowTitle; got != "Jeera" {
		t.Fatalf("WindowTitle with no active project = %q, want %q", got, "Jeera")
	}

	p := seedProject(t, st)
	p.Name = "Acme"
	if err := st.UpdateProject(p); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	m.reload()

	if got, want := m.windowTitle(), "Jeera - Acme"; got != want {
		t.Fatalf("windowTitle() = %q, want %q", got, want)
	}
	if got, want := m.View().WindowTitle, "Jeera - Acme"; got != want {
		t.Fatalf("View().WindowTitle = %q, want %q", got, want)
	}
}

// emitTabTitle writes OSC 0, which sets the terminal tab (icon name) in addition
// to the window title — Bubble Tea's WindowTitle alone only emits OSC 2.
func TestEmitTabTitle(t *testing.T) {
	got := captureOutput(t, func() {
		if msg := emitTabTitle("Jeera - Acme")(); msg != nil {
			t.Fatalf("emitTabTitle command returned %v, want nil", msg)
		}
	})
	if want := ansi.SetIconNameWindowTitle("Jeera - Acme"); got != want {
		t.Fatalf("emitTabTitle wrote %q, want %q (OSC 0)", got, want)
	}
	if !strings.HasPrefix(got, "\x1b]0;") {
		t.Fatalf("emitTabTitle should write an OSC 0 sequence, got %q", got)
	}
}

// Init emits the initial title so the tab is correct from the first frame.
func TestInitEmitsTitle(t *testing.T) {
	m, _ := newTestModel(t)
	got := captureOutput(t, func() {
		if cmd := m.Init(); cmd != nil {
			cmd()
		}
	})
	if want := ansi.SetIconNameWindowTitle(m.windowTitle()); got != want {
		t.Fatalf("Init emitted %q, want %q", got, want)
	}
}

// An update that flips the active project re-emits the tab title; one that does
// not leaves the routed command untouched.
func TestUpdateReEmitsTitleOnProjectChange(t *testing.T) {
	m, st := newTestModel(t) // starts with no project: title "Jeera"

	// A store event that surfaces a new project should re-emit the title.
	seedProject(t, st)
	next, cmd := m.Update(storeEventMsg{})
	m = next.(Model)
	if m.active.Name == "" {
		t.Fatal("project should be active after the store event")
	}
	out := captureOutput(t, func() {
		if cmd != nil {
			cmd()
		}
	})
	if want := ansi.SetIconNameWindowTitle("Jeera - Jeera"); !strings.Contains(out, want) {
		t.Fatalf("update that changed the title emitted %q, want it to contain %q", out, want)
	}

	// A subsequent no-op update must not re-emit (no OSC sequence written).
	_, cmd2 := m.Update(storeEventMsg{})
	out2 := captureOutput(t, func() {
		if cmd2 != nil {
			cmd2()
		}
	})
	if strings.Contains(out2, "\x1b]0;") {
		t.Fatalf("update with unchanged title should not emit OSC 0, got %q", out2)
	}
}

func TestViewSwitchingCycles(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m)
	if m.view != viewBoard {
		t.Fatalf("should start on the board, got view %d", m.view)
	}

	for _, want := range []view{viewBacklog, viewSprints, viewRuns, viewBoard} {
		next, _ := m.Update(keyPress("alt+tab"))
		m = next.(Model)
		if m.view != want {
			t.Errorf("tab → view %d, want %d", m.view, want)
		}
	}

	next, _ := m.Update(keyPress("alt+shift+tab"))
	m = next.(Model)
	if m.view != viewRuns {
		t.Errorf("shift+tab from board wraps → runs, got %d", m.view)
	}
}

func TestGlobalKeysReachableFromSprints(t *testing.T) {
	m, _ := newTestModel(t)
	seedSprints(t, &m) // leaves the model on the Sprints view

	for _, tc := range []struct {
		key  string
		want mode
	}{
		{"p", modeProjects},
		{",", modeSettings},
		{"?", modeHelp},
		{"m", modeMCP},
	} {
		next, _ := m.Update(keyPress(tc.key))
		if got := next.(Model).mode; got != tc.want {
			t.Errorf("%q from the Sprints view: mode = %v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestShiftTabFromSprints(t *testing.T) {
	m, _ := newTestModel(t)
	seedSprints(t, &m) // leaves the model on the Sprints view
	next, _ := m.Update(keyPress("alt+shift+tab"))
	if got := next.(Model).view; got != viewBacklog {
		t.Errorf("shift+tab from Sprints should step back to Backlog, got view %d", got)
	}
}

func TestViewSwitchPreservesBoardSelection(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m) // first column holds two issues
	m.colIdx, m.cardIdx = 0, 0

	next, _ := m.Update(keyPress("j")) // move down one card
	m = next.(Model)
	want, ok := m.selectedIssue()
	if !ok {
		t.Fatal("expected a selected issue after moving down")
	}

	for i := 0; i < int(viewCount); i++ { // a full lap returns to the board
		next, _ = m.Update(keyPress("alt+tab"))
		m = next.(Model)
	}
	if m.view != viewBoard {
		t.Fatalf("expected to land back on the board, got view %d", m.view)
	}
	got, _ := m.selectedIssue()
	if got.ID != want.ID {
		t.Errorf("selection drifted across a view round-trip: got %d want %d", got.ID, want.ID)
	}
}
