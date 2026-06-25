package tui

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"

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

// TestModalShell pins the shared dialog chrome: it renders to exactly the
// requested width, carries the title/body/hint, and zones them with two
// hairlines spanning the full inner width (width − border − padding).
func TestModalShell(t *testing.T) {
	th := theme.New()
	for _, w := range []int{56, 64, 72, 98} {
		got := modalShell(th, w, 0, "Title", "subtitle", "body line", modalHint(th, "esc close"))
		if gw := lipgloss.Width(got); gw != w {
			t.Errorf("modalShell width = %d, want %d", gw, w)
		}
		plain := stripANSI(got)
		for _, want := range []string{"Title", "subtitle", "body line", "esc close"} {
			if !strings.Contains(plain, want) {
				t.Errorf("modalShell at width %d missing %q\n%s", w, want, plain)
			}
		}
		// Two interior hairlines (header/body and body/footer), each bounded by the
		// modal's side borders — distinct from the longer top/bottom box edges.
		rule := strings.Repeat("─", w-6)
		n := 0
		for _, ln := range strings.Split(plain, "\n") {
			if s := strings.TrimSpace(ln); strings.HasPrefix(s, "│") && strings.Contains(s, rule) {
				n++
			}
		}
		if n != 2 {
			t.Errorf("modalShell at width %d: found %d interior hairlines, want 2", w, n)
		}
	}
}

// TestModalShellMinBodyHeight proves the body floor adds presence: a one-line
// body padded to a taller floor yields a correspondingly taller dialog.
func TestModalShellMinBodyHeight(t *testing.T) {
	th := theme.New()
	short := lipgloss.Height(modalShell(th, 64, 0, "T", "", "one line", ""))
	tall := lipgloss.Height(modalShell(th, 64, 8, "T", "", "one line", ""))
	if tall <= short {
		t.Errorf("minBodyH should grow the dialog: short=%d tall=%d", short, tall)
	}
}

// TestConfirmDialog covers the one centered overlay without a golden: it names
// the action, echoes the message, and offers the y/n choice at the family width.
func TestConfirmDialog(t *testing.T) {
	m, _ := newTestModel(t)
	m.confirm = "Delete JEE-1 — build the thing?"
	got := m.renderConfirm()
	if w := lipgloss.Width(got); w != modalWidthConfirm {
		t.Errorf("confirm width = %d, want %d", w, modalWidthConfirm)
	}
	plain := stripANSI(got)
	for _, want := range []string{"Confirm", "Delete JEE-1 — build the thing?", "yes", "no"} {
		if !strings.Contains(plain, want) {
			t.Errorf("confirm dialog missing %q\n%s", want, plain)
		}
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

// TestNavbar checks the header fills the width, leads with the JEERA wordmark
// on the left, names every destination, right-aligns the pills, and underlines
// with a hairline.
func TestNavbar(t *testing.T) {
	th := theme.New()
	items := []navItem{{iconBoard, "Board"}, {iconBacklog, "Backlog"}, {iconSprints, "Sprints"}, {iconRuns, "Runs"}}
	got := navbar(th, 100, items, 0, brandLogo(th))
	plain := stripANSI(got)
	if !strings.Contains(plain, "J E E R A") {
		t.Errorf("navbar missing JEERA wordmark:\n%s", plain)
	}
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

	// The wordmark must lead (sit left of every pill) and the pills must be
	// right-aligned. The dragon mosaic is taller than the pill row, so the
	// wordmark and the pill labels are centred on different rows — compare their
	// columns across the whole block rather than within one line.
	lines := strings.Split(plain, "\n")
	colOf := func(s string) int {
		for _, line := range lines {
			if i := strings.Index(line, s); i >= 0 {
				return i
			}
		}
		return -1
	}
	jeera, board := colOf("J E E R A"), colOf("Board")
	if jeera < 0 || board < 0 || jeera >= board {
		t.Errorf("wordmark should lead the pills: jeera=%d board=%d\n%s", jeera, board, plain)
	}
	var runsLine string
	for _, line := range lines {
		if strings.Contains(line, "Runs") {
			runsLine = line
			break
		}
	}
	runs := strings.Index(runsLine, "Runs")
	if tail := lipgloss.Width(runsLine) - (runs + len("Runs")); tail > 6 {
		t.Errorf("pills not right-aligned: %d cells of trailing space after Runs\n%q", tail, runsLine)
	}
}

// TestBrandLogo checks the logo always carries both the dragon mosaic and the
// JEERA wordmark — the mosaic is baked in truecolor but always emitted, since
// Bubble Tea downsamples the colours to whatever the terminal supports.
func TestBrandLogo(t *testing.T) {
	th := theme.New()
	logo := brandLogo(th)
	plain := stripANSI(logo)

	if !strings.Contains(plain, "J E E R A") {
		t.Errorf("logo missing wordmark: %q", plain)
	}
	// The mosaic is five braille rows beside the wordmark (an odd height, so the
	// 3-row nav pills centre cleanly within the band).
	if lipgloss.Height(logo) != 5 {
		t.Errorf("logo should be five rows (mosaic height), got %d", lipgloss.Height(logo))
	}
	hasBraille := false
	for _, r := range plain {
		if r >= 0x2800 && r <= 0x28FF {
			hasBraille = true
			break
		}
	}
	if !hasBraille {
		t.Errorf("logo should contain the braille dragon mosaic, got %q", plain)
	}
}

// TestLogoMosaicDownsamples proves the "render always" contract: the mosaic is
// baked in 24-bit colour, but its escapes downsample cleanly to a lesser
// profile (as Bubble Tea's renderer does at paint time), so the dragon shows on
// 256-colour terminals like Apple Terminal rather than vanishing.
func TestLogoMosaicDownsamples(t *testing.T) {
	var buf bytes.Buffer
	w := colorprofile.Writer{Forward: &buf, Profile: colorprofile.ANSI256}
	if _, err := w.WriteString(logoMosaic); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "\x1b[38;2;") {
		t.Error("24-bit colour codes should be gone after ANSI256 downsampling")
	}
	if !strings.Contains(out, "\x1b[38;5;") {
		t.Error("expected 256-colour codes after downsampling the mosaic")
	}
}

// TestDumpComponents is a visual aid: run `go test ./internal/tui -run
// DumpComponents -v` to eyeball the primitives. It asserts nothing.
func TestDumpComponents(t *testing.T) {
	th := theme.New()
	items := []navItem{{iconBoard, "Board"}, {iconBacklog, "Backlog"}, {iconSprints, "Sprints"}, {iconRuns, "Runs"}}
	t.Log("\n" + navbar(th, 80, items, 2, brandLogo(th)))
	body := "Status    ◀ In Progress ▶\nType      Task\nPriority  ▲ High\nPoints    5\nSprint    Sprint 3\nTags      #ui #infra"
	t.Log("\n" + panel(th, panelOpts{title: "Properties", body: body, width: 36, height: 9, focused: true}))
	t.Log("\n" + panel(th, panelOpts{title: "Agent", body: "claude · opus · high\n\n" + buttonRow(th, []string{iconRun + " Run", iconDiscuss + " Discuss", iconClock + " Schedule"}, 0), width: 44, height: 6, focused: false}))
	left := []cell{cText("● "), cFg("▲ ", th.P.Danger), cKey("JEE-12   ", th.P.Focus), cText("Fix the card overflow on narrow widths")}
	t.Log("\n" + listRow(th, 70, true, left, []cell{cText("5pt")}) + "\n" + listRow(th, 70, false, left, []cell{cText("3pt")}))
}
