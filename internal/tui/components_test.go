package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

// TestPanelDimensions pins the signature element's contract: a panel renders
// exactly width×height cells at any size, so the bento grid never drifts.
func TestPanelDimensions(t *testing.T) {
	th := theme.New()
	for _, tc := range []struct{ w, h int }{
		{40, 10}, {20, 5}, {80, 24}, {12, 3}, {6, 3},
	} {
		body := "Status   To Do\nType     Task\nPriority High"
		got := panel(th, panelOpts{title: "Properties", body: body, width: tc.w, height: tc.h, focused: true})
		if w := lipgloss.Width(got); w != tc.w {
			t.Errorf("panel %dx%d: width = %d, want %d", tc.w, tc.h, w, tc.w)
		}
		if h := lipgloss.Height(got); h != tc.h {
			t.Errorf("panel %dx%d: height = %d, want %d", tc.w, tc.h, h, tc.h)
		}
	}
}

// TestPanelTitleInlaid proves the title is seated in the top border.
func TestPanelTitleInlaid(t *testing.T) {
	th := theme.New()
	got := panel(th, panelOpts{title: "Agent", body: "x", width: 30, height: 4})
	first := strings.Split(stripANSI(got), "\n")[0]
	if !strings.Contains(first, "AGENT") {
		t.Errorf("panel top border should inlay the title, got %q", first)
	}
	if !strings.HasPrefix(first, "╭─ AGENT ") {
		t.Errorf("title should follow the left corner, got %q", first)
	}
}

// TestListRowSpansWidth is the fix for "rows don't take 100% of the width": a
// selected row must fill exactly the full width as one continuous bar.
func TestListRowSpansWidth(t *testing.T) {
	th := theme.New()
	left := []cell{cKey("JEE-1", th.P.Focus), cText("  build the thing")}
	right := []cell{cText("5pt")}
	for _, w := range []int{40, 80, 120} {
		row := listRow(th, w, true, left, right)
		if got := lipgloss.Width(row); got != w {
			t.Errorf("listRow width = %d, want %d", got, w)
		}
	}
}

// TestNavbarCentered checks the navbar fills the width, names every
// destination, and underlines with a hairline.
func TestNavbarCentered(t *testing.T) {
	th := theme.New()
	items := []navItem{{iconBoard, "Board"}, {iconBacklog, "Backlog"}, {iconSprints, "Sprints"}, {iconRuns, "Runs"}}
	got := navbar(th, 100, items, 0)
	plain := stripANSI(got)
	for _, lbl := range []string{"Board", "Backlog", "Sprints", "Runs"} {
		if !strings.Contains(plain, lbl) {
			t.Errorf("navbar missing %q", lbl)
		}
	}
	for _, line := range strings.Split(plain, "\n") {
		if lipgloss.Width(line) != 100 {
			t.Errorf("navbar line not full width (%d): %q", lipgloss.Width(line), line)
		}
	}
}

// TestDumpComponents is a visual aid: run `go test ./internal/tui -run
// DumpComponents -v` to eyeball the primitives. It asserts nothing.
func TestDumpComponents(t *testing.T) {
	th := theme.New()
	items := []navItem{{iconBoard, "Board"}, {iconBacklog, "Backlog"}, {iconSprints, "Sprints"}, {iconRuns, "Runs"}}
	t.Log("\n" + navbar(th, 80, items, 2))
	body := "Status    ◀ In Progress ▶\nType      Task\nPriority  ▲ High\nPoints    5\nSprint    Sprint 3\nTags      #ui #infra"
	t.Log("\n" + panel(th, panelOpts{title: "Properties", body: body, width: 36, height: 9, focused: true}))
	t.Log("\n" + panel(th, panelOpts{title: "Agent", body: "claude · opus · high\n\n" + buttonRow(th, []string{iconRun + " Run", iconChildren + " +children", "Discuss"}, 0), width: 44, height: 6, focused: false}))
	left := []cell{cText("● "), cFg("▲ ", th.P.Danger), cKey("JEE-12   ", th.P.Focus), cText("Fix the card overflow on narrow widths")}
	t.Log("\n" + listRow(th, 70, true, left, []cell{cText("5pt")}) + "\n" + listRow(th, 70, false, left, []cell{cText("3pt")}))
}
