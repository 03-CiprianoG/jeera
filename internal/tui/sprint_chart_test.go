package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

func TestMeter(t *testing.T) {
	th := theme.New()
	cases := []struct {
		value, total, width int
		wantFilled          int
	}{
		{0, 10, 10, 0},   // empty
		{5, 10, 10, 5},   // half
		{10, 10, 10, 10}, // full
		{15, 10, 10, 10}, // clamps to full
		{1, 30, 10, 1},   // a sliver shows for any progress
		{0, 0, 10, 0},    // no total → empty, no divide-by-zero
	}
	for _, c := range cases {
		got := stripANSI(meter(th, c.value, c.total, c.width, th.P.Success))
		filled := strings.Count(got, "█")
		track := strings.Count(got, "░")
		if filled != c.wantFilled {
			t.Errorf("meter(%d/%d, w=%d): filled=%d want %d", c.value, c.total, c.width, filled, c.wantFilled)
		}
		if filled+track != c.width {
			t.Errorf("meter(%d/%d, w=%d): filled+track=%d want %d", c.value, c.total, c.width, filled+track, c.width)
		}
	}
}

func TestBrailleCanvasSet(t *testing.T) {
	c := newBrailleCanvas(2, 2) // 4×8 dots
	c.set(0, 0)                 // top-left dot → bit 0x01
	if c.bits[0][0] != 0x01 {
		t.Errorf("set(0,0): bits=%#x want 0x01", c.bits[0][0])
	}
	c.set(1, 3) // dx=1,dy=3 → bit 0x80, lower-right of the same cell
	if c.bits[0][0]&0x80 == 0 {
		t.Errorf("set(1,3): bit 0x80 not set, bits=%#x", c.bits[0][0])
	}
	// Out-of-bounds dots are ignored rather than panicking.
	c.set(99, 99)
	c.set(-1, -1)
}

func TestBrailleCanvasLine(t *testing.T) {
	c := newBrailleCanvas(2, 2) // dots 0..3 × 0..7
	c.line(0, 0, c.dotW()-1, c.dotH()-1)
	if c.bits[0][0]&0x01 == 0 {
		t.Errorf("diagonal should light the start dot, bits[0][0]=%#x", c.bits[0][0])
	}
	if c.bits[1][1]&0x80 == 0 {
		t.Errorf("diagonal should light the end dot, bits[1][1]=%#x", c.bits[1][1])
	}
}

func TestMergeCanvasesEmptyIsBlank(t *testing.T) {
	a := newBrailleCanvas(3, 1)
	b := newBrailleCanvas(3, 1)
	rows := mergeCanvases(a, b, theme.New().P.Success, theme.New().P.TextSubtle)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if stripANSI(rows[0]) != "   " {
		t.Errorf("empty canvases should render spaces, got %q", stripANSI(rows[0]))
	}
}

// hasBraille reports whether s contains any braille-pattern rune (U+2800–U+28FF),
// i.e. the chart actually plotted a line.
func hasBraille(s string) bool {
	for _, r := range s {
		if r >= 0x2800 && r <= 0x28FF && r != 0x2800 {
			return true
		}
	}
	return false
}

func TestRenderBurndownEmptyStates(t *testing.T) {
	th := theme.New()
	loc := time.UTC
	start := mustDayUTC(t, "2026-06-20")
	end := mustDayUTC(t, "2026-07-04")
	now := mustDayUTC(t, "2026-06-27")

	// Future sprint: not started → an explanation, no plotted line.
	future := core.Sprint{Name: "S", State: core.SprintFuture, StartAt: &start, EndAt: &end}
	mt := computeSprintMetrics(future, nil, testStatuses(), now)
	out := stripANSI(renderBurndown(th, future, mt, loc, 40, 12))
	if hasBraille(out) {
		t.Error("a future sprint should not plot a burndown line")
	}
	if !strings.Contains(out, "starts") {
		t.Errorf("future burndown should explain itself, got:\n%s", out)
	}

	// Active but undated → asks for dates.
	undated := core.Sprint{Name: "S", State: core.SprintActive}
	mt = computeSprintMetrics(undated, nil, testStatuses(), now)
	out = stripANSI(renderBurndown(th, undated, mt, loc, 40, 12))
	if !strings.Contains(out, "dates") {
		t.Errorf("undated burndown should ask for dates, got:\n%s", out)
	}
}

func TestRenderBurndownStructure(t *testing.T) {
	th := theme.New()
	loc := time.UTC
	start := mustDayUTC(t, "2026-06-20")
	end := mustDayUTC(t, "2026-07-04")
	now := mustDayUTC(t, "2026-06-27")
	sp := core.Sprint{Name: "S", State: core.SprintActive, StartAt: &start, EndAt: &end}

	issues := []core.Issue{
		doneIssue(1, 5, "2026-06-22"),
		doneIssue(2, 3, "2026-06-25"),
		{ID: 3, StatusID: 2, StoryPoints: ptr(8)}, // in progress, unfinished
	}
	mt := computeSprintMetrics(sp, issues, testStatuses(), now)

	const w, h = 44, 14
	out := renderBurndown(th, sp, mt, loc, w, h)
	plain := stripANSI(out)
	lines := strings.Split(plain, "\n")
	if len(lines) != h {
		t.Fatalf("burndown should be exactly %d rows, got %d:\n%s", h, len(lines), plain)
	}
	if !hasBraille(plain) {
		t.Error("an active dated sprint should plot a braille line")
	}
	// The scope (16 pts) and zero tick label the y-axis; the window's dates label
	// the x-axis.
	if !strings.Contains(plain, "16") {
		t.Errorf("y-axis should show the scope (16), got:\n%s", plain)
	}
	if !strings.Contains(plain, "Jun 20") || !strings.Contains(plain, "Jul 4") {
		t.Errorf("x-axis should show the window dates, got:\n%s", plain)
	}
	for i, ln := range lines {
		if lineWidth(ln) > w {
			t.Errorf("row %d overflows width %d: %q (%d)", i, w, ln, lineWidth(ln))
		}
	}
}

// --- shared test helpers for sprint metrics/rendering ------------------------

func mustDayUTC(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return tm
}

func ptr(n int) *int { return &n }

// testStatuses mirrors a project's four default columns, keyed by id, so
// in-memory issues can map to a category without a store.
func testStatuses() map[int64]core.Status {
	return map[int64]core.Status{
		1: {ID: 1, Name: "To Do", Category: core.CategoryTodo, Position: 0},
		2: {ID: 2, Name: "In Progress", Category: core.CategoryInProgress, Position: 1},
		3: {ID: 3, Name: "In Review", Category: core.CategoryReview, Position: 2},
		4: {ID: 4, Name: "Done", Category: core.CategoryDone, Position: 3},
	}
}

// doneIssue is a Done issue (status 4) of pts points, last updated on day — the
// signal the burndown reconstruction reads as its completion.
func doneIssue(id int64, pts int, day string) core.Issue {
	tm, _ := time.Parse("2006-01-02", day)
	return core.Issue{ID: id, StatusID: 4, StoryPoints: &pts, UpdatedAt: tm}
}
