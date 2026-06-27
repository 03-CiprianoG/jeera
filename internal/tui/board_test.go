package tui

import (
	"fmt"
	"testing"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// TestCardWindow pins the height-aware windowing the board uses to keep the
// selected card on screen: whole cards only, scrolling no further than needed,
// and the cursor always inside the returned range.
func TestCardWindow(t *testing.T) {
	uniform := make([]int, 10) // ten 4-row cards
	for i := range uniform {
		uniform[i] = 4
	}

	cases := []struct {
		name             string
		heights          []int
		cursor, budget   int
		wantStart, wantE int
	}{
		{"everything fits", []int{4, 4}, 0, 100, 0, 2},
		{"top of a long lane", uniform, 0, 9, 0, 2},
		{"cursor rides the bottom edge", uniform, 4, 9, 3, 5},
		{"bottom of the lane", uniform, 9, 9, 8, 10},
		{"variable heights stay whole", []int{5, 4, 3, 5, 4}, 2, 9, 1, 3},
		{"budget below one card still shows the cursor", uniform, 5, 2, 5, 6},
		{"cursor past the end clamps", uniform, 99, 9, 8, 10},
		{"empty", nil, 0, 10, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, end := cardWindow(c.heights, c.cursor, c.budget)
			if start != c.wantStart || end != c.wantE {
				t.Fatalf("cardWindow(%v, %d, %d) = (%d, %d), want (%d, %d)",
					c.heights, c.cursor, c.budget, start, end, c.wantStart, c.wantE)
			}
			if n := len(c.heights); n > 0 {
				cur := min(max(c.cursor, 0), n-1)
				if cur < start || cur >= end {
					t.Errorf("cursor %d (clamped %d) fell outside window [%d, %d)", c.cursor, cur, start, end)
				}
			}
		})
	}
}

// seedTallColumn fills the To Do lane with more issues than fit at the test's
// fixed 100x30, forcing the column to scroll. New issues land in the project's
// initial (To Do) status, so they stack in column 0.
func seedTallColumn(t *testing.T, m *Model) {
	t.Helper()
	p := seedProject(t, m.store)
	sp, err := m.store.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "Sprint 1", State: core.SprintActive})
	if err != nil {
		t.Fatalf("CreateSprint: %v", err)
	}
	for i := 1; i <= 10; i++ {
		iss, err := m.store.CreateIssue(core.Issue{
			ProjectID: p.ID,
			Title:     fmt.Sprintf("Backlog item number %d", i),
			Type:      core.TypeTask,
			Priority:  core.PriorityMedium,
		})
		if err != nil {
			t.Fatalf("CreateIssue %d: %v", i, err)
		}
		// Scope each to the active sprint so the board (not the backlog) holds them.
		if err := m.store.AddIssueToSprint(iss.ID, &sp.ID); err != nil {
			t.Fatalf("AddIssueToSprint %d: %v", i, err)
		}
	}
	m.reload()
}

// down walks the board cursor down n rows the way a key press would, so the
// scroll goldens exercise the real selection path (and its clamping).
func boardDown(t *testing.T, m Model, n int) Model {
	t.Helper()
	for i := 0; i < n; i++ {
		next, _ := m.updateBoard(keyPress("down"))
		m = next.(Model)
	}
	return m
}

// TestGoldenBoardScrollTop: a long To Do lane parked at the top shows a "N more"
// hint at the bottom edge and no hint at the top.
func TestGoldenBoardScrollTop(t *testing.T) {
	m, _ := newTestModel(t)
	seedTallColumn(t, &m)
	m.colIdx, m.cardIdx = 0, 0
	goldenFile(t, "board_scroll_top", render(m))
}

// TestGoldenBoardScrollMiddle: with the cursor walked into the middle of the
// lane, both edges carry a "N more" hint and the selection stays on screen.
func TestGoldenBoardScrollMiddle(t *testing.T) {
	m, _ := newTestModel(t)
	seedTallColumn(t, &m)
	m.colIdx, m.cardIdx = 0, 0
	m = boardDown(t, m, 6)
	goldenFile(t, "board_scroll_middle", render(m))
}

// TestGoldenBoardScrollBottom: pressing down past the last card lands on the
// "+ New issue" slot; the lane scrolls to reveal it with only a top hint.
func TestGoldenBoardScrollBottom(t *testing.T) {
	m, _ := newTestModel(t)
	seedTallColumn(t, &m)
	m.colIdx, m.cardIdx = 0, 0
	m = boardDown(t, m, 12) // clamps onto the add slot
	goldenFile(t, "board_scroll_bottom", render(m))
}
