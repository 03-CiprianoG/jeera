package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// --- matching ----------------------------------------------------------------

func TestMatchesQuery(t *testing.T) {
	iss := core.Issue{
		Key: "JEE-12", Title: "Fix the login flow", Description: "OAuth token refresh",
		Type: core.TypeBug, Assignee: core.Assignee{Model: "opus"},
	}
	cases := []struct {
		query string
		want  bool
		why   string
	}{
		{"", true, "a blank query matches everything"},
		{"login", true, "title substring"},
		{"LOGIN", true, "match is case-insensitive"},
		{"jee-12", true, "key, lowercased"},
		{"oauth", true, "description text"},
		{"bug", true, "issue type"},
		{"opus", true, "assignee model"},
		{"login flow", true, "every term present (AND)"},
		{"login logout", false, "one term missing fails the whole query"},
		{"  fix   token  ", true, "terms span fields and extra spaces are ignored"},
		{"fixthe", false, "terms don't span a field boundary"},
	}
	for _, c := range cases {
		if got := matchesQuery(iss, c.query); got != c.want {
			t.Errorf("matchesQuery(%q) = %v, want %v — %s", c.query, got, c.want, c.why)
		}
	}
}

func TestFilterIssues(t *testing.T) {
	issues := []core.Issue{
		{Key: "A-1", Title: "Add search"},
		{Key: "A-2", Title: "Fix search bar"},
		{Key: "A-3", Title: "Polish the navbar"},
	}
	got := filterIssues(issues, "search")
	if len(got) != 2 || got[0].Key != "A-1" || got[1].Key != "A-2" {
		t.Errorf("filterIssues(search) = %v, want A-1,A-2 in order", keysOf(got))
	}
	if all := filterIssues(issues, "   "); len(all) != 3 {
		t.Errorf("a blank query should return all issues, got %d", len(all))
	}
}

func TestFilterBoardKeepsShape(t *testing.T) {
	bd := boardData{
		columns: []column{
			{status: core.Status{Name: "To Do"}, cards: []core.Issue{{Key: "A-1", Title: "search this"}, {Key: "A-2", Title: "not me"}}},
			{status: core.Status{Name: "Done"}, cards: []core.Issue{{Key: "A-3", Title: "search done"}}},
		},
		tags: map[int64][]string{1: {"ui"}},
	}
	got := filterBoard(bd, "search")
	if len(got.columns) != 2 {
		t.Fatalf("filterBoard should preserve the lanes, got %d columns", len(got.columns))
	}
	if len(got.columns[0].cards) != 1 || got.columns[0].cards[0].Key != "A-1" {
		t.Errorf("To Do should keep only the matching card, got %v", keysOf(got.columns[0].cards))
	}
	if got.columns[0].status.Name != "To Do" {
		t.Error("filterBoard dropped a column's status")
	}
	if got.tags == nil {
		t.Error("filterBoard dropped the tag lookup")
	}
	if countCards(filterBoard(bd, "")) != 3 {
		t.Error("a blank query should leave the board whole")
	}
}

func TestMatchSummary(t *testing.T) {
	for _, c := range []struct {
		n    int
		want string
	}{{0, "no matches"}, {1, "1 match"}, {7, "7 matches"}} {
		if got := matchSummary(c.n); got != c.want {
			t.Errorf("matchSummary(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

// TestSearchIconWidth holds the icon set's one-cell contract: a wider glyph
// would shear every layout that seats the magnifier.
func TestSearchIconWidth(t *testing.T) {
	if w := lipgloss.Width(iconSearch); w != 1 {
		t.Errorf("iconSearch %q must be one cell wide, got %d", iconSearch, w)
	}
}

// --- opening -----------------------------------------------------------------

func TestSearchOpensOnBoardAndBacklog(t *testing.T) {
	for _, c := range []struct {
		key  string
		view view
	}{
		{"super+f", viewBoard}, // ⌘F — the headline, on terminals that forward the Command key
		{"/", viewBoard},       // the universal fallback
		{"super+f", viewBacklog},
		{"/", viewBacklog},
	} {
		m, _ := newTestModel(t)
		seedBoard(t, &m) // seedBoard's issues are unsprinted, so they fill the backlog too
		m.view = c.view
		m.refreshView()

		next, _ := m.Update(keyPress(c.key))
		m = next.(Model)
		if m.mode != modeSearch || m.search == nil {
			t.Fatalf("%q on %v should open search, got mode=%v", c.key, c.view, m.mode)
		}
		if m.search.target != c.view {
			t.Errorf("%q on %v opened search targeting %v", c.key, c.view, m.search.target)
		}
	}
}

func TestSearchNotOnSprintsOrRuns(t *testing.T) {
	for _, v := range []view{viewSprints, viewRuns} {
		m, _ := newTestModel(t)
		seedBoard(t, &m)
		m.view = v
		m.refreshView()
		next, _ := m.Update(keyPress("/"))
		m = next.(Model)
		if m.mode == modeSearch {
			t.Errorf("/ on %v should not open search — it's only for the Board and Backlog", v)
		}
	}
}

func TestSearchNotOnWelcome(t *testing.T) {
	m, _ := newTestModel(t) // no project
	next, _ := m.Update(keyPress("/"))
	m = next.(Model)
	if m.mode == modeSearch {
		t.Error("/ before any project loads should be a no-op")
	}
}

func TestSearchPrefillsAppliedQuery(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m)
	m.boardQuery = "the"
	m.reload()

	next, _ := m.openSearch()
	m = next.(Model)
	if got := m.search.input.Value(); got != "the" {
		t.Errorf("reopening search should pre-fill the live query, got %q", got)
	}
}

// --- applying / clearing -----------------------------------------------------

func TestSearchApplyFiltersBoard(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m)

	next, _ := m.Update(keyPress("/"))
	m = next.(Model)
	m.search.input.SetValue("the") // matches 4 of the 5 seeded titles
	next, _ = m.Update(keyPress("enter"))
	m = next.(Model)

	if m.mode != modeNormal || m.search != nil {
		t.Fatalf("applying should close the overlay, got mode=%v", m.mode)
	}
	if m.boardQuery != "the" {
		t.Errorf("boardQuery = %q, want %q", m.boardQuery, "the")
	}
	if got := countCards(m.board); got != 4 {
		t.Errorf("filtered board should hold 4 cards, got %d", got)
	}
	if m.boardTotal != 5 {
		t.Errorf("boardTotal should keep the unfiltered count 5, got %d", m.boardTotal)
	}
	if _, ok := m.selectedIssue(); !ok {
		t.Error("applying a filter should re-anchor the selection onto a match")
	}
}

func TestSearchApplyFiltersBacklog(t *testing.T) {
	m, _ := newTestModel(t)
	seedBacklog(t, &m) // "Add OAuth login" + "Fix the flaky test"

	next, _ := m.Update(keyPress("/"))
	m = next.(Model)
	m.search.input.SetValue("oauth")
	next, _ = m.Update(keyPress("enter"))
	m = next.(Model)

	if m.backlogQuery != "oauth" {
		t.Errorf("backlogQuery = %q, want %q", m.backlogQuery, "oauth")
	}
	if got := len(m.backlog.issues); got != 1 {
		t.Fatalf("filtered backlog should hold 1 issue, got %d", got)
	}
	if m.backlogTotal != 2 {
		t.Errorf("backlogTotal should keep the unfiltered count 2, got %d", m.backlogTotal)
	}
}

func TestSearchClearButtonLiftsFilter(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m)
	m.boardQuery = "the"
	m.reload()

	next, _ := m.openSearch() // pre-filled with "the"
	m = next.(Model)
	next, _ = m.Update(keyPress("tab")) // input → Apply
	m = next.(Model)
	next, _ = m.Update(keyPress("tab")) // Apply → Clear
	m = next.(Model)
	if !m.search.onClear() {
		t.Fatalf("two tabs from the input should land on Clear, focus=%d", m.search.focus)
	}
	next, _ = m.Update(keyPress("enter")) // Clear
	m = next.(Model)

	if m.boardQuery != "" {
		t.Errorf("Clear should lift the filter, boardQuery=%q", m.boardQuery)
	}
	if got := countCards(m.board); got != 5 {
		t.Errorf("clearing should restore the whole board (5), got %d", got)
	}
}

func TestSearchEscCancelKeepsAppliedQuery(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m)
	m.boardQuery = "the"
	m.reload()

	next, _ := m.openSearch()
	m = next.(Model)
	m.search.input.SetValue("rewritten") // user edits, then changes their mind
	next, _ = m.Update(keyPress("esc"))  // cancel
	m = next.(Model)

	if m.mode != modeNormal || m.search != nil {
		t.Fatalf("esc should close the overlay, mode=%v", m.mode)
	}
	if m.boardQuery != "the" {
		t.Errorf("cancelling must not touch the live filter, boardQuery=%q", m.boardQuery)
	}
}

func TestSearchEscClearsActiveFilter(t *testing.T) {
	for _, v := range []view{viewBoard, viewBacklog} {
		m, _ := newTestModel(t)
		seedBoard(t, &m)
		m.view = v
		if v == viewBoard {
			m.boardQuery = "the"
		} else {
			m.backlogQuery = "the"
		}
		m.reload() // reload also refreshes the active view

		next, _ := m.Update(keyPress("esc"))
		m = next.(Model)
		if m.boardQuery != "" || m.backlogQuery != "" {
			t.Errorf("esc on %v should lift the live filter, board=%q backlog=%q", v, m.boardQuery, m.backlogQuery)
		}
	}
}

// TestSearchKeystrokesStayInOverlay proves the overlay owns the keyboard while
// open: a printable key edits the query rather than leaking to the board.
func TestSearchKeystrokesStayInOverlay(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m)
	next, _ := m.Update(keyPress("/"))
	m = next.(Model)
	next, _ = m.Update(keyPress("x")) // would normally delete a card on the board
	m = next.(Model)
	if m.mode != modeSearch {
		t.Errorf("a keystroke while searching must stay in the overlay, mode=%v", m.mode)
	}
}

// --- live count --------------------------------------------------------------

func TestSearchLiveCount(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m)
	next, _ := m.openSearch()
	m = next.(Model)

	m.search.input.SetValue("the")
	if got := m.search.matchCount(); got != 4 {
		t.Errorf("live count for %q = %d, want 4", "the", got)
	}
	m.search.input.SetValue("zzz")
	if got := m.search.matchCount(); got != 0 {
		t.Errorf("live count for a miss = %d, want 0", got)
	}
}

// --- view integration --------------------------------------------------------

func TestSearchHidesAddCardWhileFiltering(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m)
	if !m.columnHasAddCard(0) {
		t.Fatal("the To Do column should show the add-card when not filtering")
	}
	m.boardQuery = "the"
	m.reload()
	if m.columnHasAddCard(0) {
		t.Error("the add-card should be hidden while a board search is live")
	}
}

func TestSearchPersistsAcrossReload(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m)
	m.boardQuery = "the"
	m.reload()
	before := countCards(m.board)

	m.reload() // a store event fires this; the filter must survive
	if got := countCards(m.board); got != before {
		t.Errorf("the filter should persist across reload: %d then %d", before, got)
	}
	if m.boardTotal != 5 {
		t.Errorf("boardTotal should stay the unfiltered 5 across reload, got %d", m.boardTotal)
	}
}

func TestFilterHeaderShowsScopeQueryAndCount(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m)
	m.boardQuery = "the"
	m.reload()

	out := render(m)
	if !strings.Contains(out, iconSearch) {
		t.Error("an active board search should show the magnifier")
	}
	if !strings.Contains(out, "BOARD") {
		t.Errorf("the filter header should name the scope it filters:\n%s", out)
	}
	if !strings.Contains(out, "4 of 5") {
		t.Errorf("the filter header should read '4 of 5':\n%s", out)
	}
	if !strings.Contains(out, "esc clears") {
		t.Error("the filter header should advertise how to clear it")
	}
}

func TestSearchEmptyState(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m)
	m.boardQuery = "zzzzz"
	m.reload()

	out := render(m)
	if !strings.Contains(out, "No matches") {
		t.Errorf("a zero-match filter should show a dead-end:\n%s", out)
	}
	if !strings.Contains(out, "zzzzz") {
		t.Error("the dead-end should name the query that found nothing")
	}
}

func TestBacklogHeaderKeepsTrueTotalWhileFiltering(t *testing.T) {
	m, _ := newTestModel(t)
	seedBacklog(t, &m) // 2 unsprinted issues
	m.backlogQuery = "oauth"
	m.refreshView()

	out := render(m)
	// The filter header folds the backlog's true size into its "N of M" count, so
	// the total stays in view without stacking a separate "unsprinted" line.
	if !strings.Contains(out, "1 of 2") {
		t.Errorf("the filter header should read '1 of 2', keeping the true total in view:\n%s", out)
	}
	if !strings.Contains(out, "BACKLOG") {
		t.Errorf("the filter header should keep naming the scope it filters:\n%s", out)
	}
	if strings.Contains(out, "unsprinted") {
		t.Errorf("the filter header replaces the plain heading — no stacked 'unsprinted' line:\n%s", out)
	}
}

// --- golden ------------------------------------------------------------------

func TestGoldenSearch(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m)
	next, _ := m.openSearch()
	m = next.(Model)
	m.search.input.SetValue("the")
	goldenFile(t, "search", render(m))
}

func TestGoldenBoardSearch(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m)
	m.boardQuery = "the"
	m.reload()
	m.colIdx, m.cardIdx = 0, 0
	goldenFile(t, "board_search", render(m))
}

func TestGoldenBoardSearchEmpty(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m)
	m.boardQuery = "zzzzz"
	m.reload()
	goldenFile(t, "board_search_empty", render(m))
}

func TestGoldenBacklogSearch(t *testing.T) {
	m, _ := newTestModel(t)
	seedBacklog(t, &m)
	m.backlogQuery = "fix"
	m.refreshView()
	m.backlogSel = 0
	goldenFile(t, "backlog_search", render(m))
}

// keysOf lists issue keys, for terse failure messages.
func keysOf(issues []core.Issue) []string {
	ks := make([]string, len(issues))
	for i, iss := range issues {
		ks[i] = iss.Key
	}
	return ks
}
