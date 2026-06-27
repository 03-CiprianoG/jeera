package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

// search.go is Jeera's in-view find: a ⌘F / ctrl+f / "/" overlay on the Board
// and Backlog that narrows the visible issues to those matching a query, plus
// the section header that marks a filter as live. Matching, the overlay model
// and the shared chrome live together here so the two views stay congruent.

// --- matching ----------------------------------------------------------------

// searchHaystack is the lowercased, NUL-joined text an issue is matched
// against: its key, title, description, type and assignee model. Joining on a
// NUL keeps a query term from spanning two fields (so "fix login" can't match
// "…fix" + "login…" across a boundary).
func searchHaystack(iss core.Issue) string {
	return strings.ToLower(strings.Join([]string{
		iss.Key, iss.Title, iss.Description, string(iss.Type), iss.Assignee.Model,
	}, "\x00"))
}

// matchesQuery reports whether an issue satisfies a query. The query is split
// into whitespace-separated terms and every term must appear (case-insensitive)
// — so "auth bug" finds the issues mentioning both, the robust default for a
// refining search. A blank query matches everything.
func matchesQuery(iss core.Issue, query string) bool {
	hay := searchHaystack(iss)
	for _, term := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(hay, term) {
			return false
		}
	}
	return true
}

// filterIssues returns the issues matching query in their original order. A
// blank query returns the input unchanged.
func filterIssues(issues []core.Issue, query string) []core.Issue {
	if strings.TrimSpace(query) == "" {
		return issues
	}
	out := make([]core.Issue, 0, len(issues))
	for _, iss := range issues {
		if matchesQuery(iss, query) {
			out = append(out, iss)
		}
	}
	return out
}

// filterBoard returns a copy of bd whose columns keep only the cards matching
// query; the lanes themselves (and the tag lookup) are preserved, so the board
// holds its shape while a search thins what sits in each one. A blank query
// returns bd unchanged.
func filterBoard(bd boardData, query string) boardData {
	if strings.TrimSpace(query) == "" {
		return bd
	}
	cols := make([]column, len(bd.columns))
	for i, c := range bd.columns {
		cols[i] = column{status: c.status, cards: filterIssues(c.cards, query)}
	}
	return boardData{sprint: bd.sprint, columns: cols, tags: bd.tags}
}

// countCards totals the cards across a board's columns.
func countCards(bd boardData) int {
	n := 0
	for _, c := range bd.columns {
		n += len(c.cards)
	}
	return n
}

// matchSummary phrases a result count for the overlay: "no matches", "1 match",
// or "N matches".
func matchSummary(n int) string {
	switch n {
	case 0:
		return "no matches"
	case 1:
		return "1 match"
	default:
		return fmt.Sprintf("%d matches", n)
	}
}

// searchScope names the view a search filters, for the overlay heading.
func searchScope(v view) string {
	if v == viewBacklog {
		return "Backlog"
	}
	return "Board"
}

// --- overlay model -----------------------------------------------------------

// searchModel backs the search overlay: one query input with live match
// feedback and the two actions the user asked for — Apply and Clear. It carries
// a snapshot of the target view's issues taken when the overlay opened, so the
// count updates as the user types without touching the store on every keystroke.
type searchModel struct {
	target view            // viewBoard or viewBacklog — what this filter narrows
	input  textinput.Model // the query field
	all    []core.Issue    // unfiltered snapshot, for the live count
	focus  int             // 0 input · 1 Apply · 2 Clear
}

func (s *searchModel) focusCount() int { return 3 }
func (s *searchModel) onApply() bool   { return s.focus == 1 }
func (s *searchModel) onClear() bool   { return s.focus == 2 }

// draft is the trimmed query as typed so far.
func (s *searchModel) draft() string { return strings.TrimSpace(s.input.Value()) }

// matchCount counts the snapshot issues satisfying the current draft.
func (s *searchModel) matchCount() int {
	n := 0
	for _, iss := range s.all {
		if matchesQuery(iss, s.input.Value()) {
			n++
		}
	}
	return n
}

// buttonFocus maps the focus index to the focused button (0 Apply, 1 Clear), or
// -1 while the input is focused.
func (s *searchModel) buttonFocus() int {
	switch {
	case s.onApply():
		return 0
	case s.onClear():
		return 1
	default:
		return -1
	}
}

// syncFocus focuses the input only while the cursor is on it, so a focused
// button shows no caret.
func (s *searchModel) syncFocus() tea.Cmd {
	if s.focus == 0 {
		return s.input.Focus()
	}
	s.input.Blur()
	return nil
}

// update routes a message (typing, cursor blink) to the query input.
func (s *searchModel) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	return cmd
}

// openSearch raises the search overlay for the active view, pre-filled with any
// query already applied there so the user refines rather than retypes. It is a
// no-op on views without search (Sprints, Runs) and before a project is loaded.
func (m Model) openSearch() (tea.Model, tea.Cmd) {
	if m.active.ID == 0 {
		return m, nil
	}
	var all []core.Issue
	var applied string
	switch m.view {
	case viewBacklog:
		applied = m.backlogQuery
		if bl, err := loadBacklog(m.store, m.active.ID); err == nil {
			all = bl.issues
		}
	case viewBoard:
		applied = m.boardQuery
		if bd, err := loadBoard(m.store, m.active.ID); err == nil {
			if bd.sprint == nil {
				return m, nil // no active sprint → nothing on the board to search
			}
			for _, c := range bd.columns {
				all = append(all, c.cards...)
			}
		}
	default:
		return m, nil
	}

	ti := textinput.New()
	ti.Placeholder = "key, title, type or assignee…"
	ti.Prompt = ""
	ti.CharLimit = 120
	ti.SetWidth(52)
	ti.SetValue(applied)

	s := &searchModel{target: m.view, input: ti, all: all}
	m.search = s
	m.mode = modeSearch
	return m, s.syncFocus()
}

// updateSearch drives the search overlay: tab walks the input and the two
// buttons, ←/→ moves the caret (or hops between buttons), Enter applies the
// query — or clears it from the Clear button — and esc cancels without touching
// the live filter.
func (m Model) updateSearch(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := m.search
	if s == nil {
		m.mode = modeNormal
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.closeSearch()
		return m, nil
	case "tab", "down":
		s.focus = (s.focus + 1) % s.focusCount()
		return m, s.syncFocus()
	case "shift+tab", "up":
		s.focus = (s.focus - 1 + s.focusCount()) % s.focusCount()
		return m, s.syncFocus()
	case "left", "right":
		if s.focus == 0 {
			return m, s.update(msg) // editing: move the caret
		}
		if msg.String() == "right" && s.onApply() {
			s.focus = 2
		} else if msg.String() == "left" && s.onClear() {
			s.focus = 1
		}
		return m, s.syncFocus()
	case "enter":
		if s.onClear() {
			return m.applySearch(s.target, "")
		}
		return m.applySearch(s.target, s.input.Value())
	}
	if s.focus == 0 {
		return m, s.update(msg)
	}
	return m, nil
}

// applySearch commits query as the live filter for view v, closes the overlay
// and re-anchors the selection to the first match. A blank query lifts the
// filter. It is called only for the view currently showing, so reload/refreshView
// re-filter the right data.
func (m Model) applySearch(v view, query string) (tea.Model, tea.Cmd) {
	q := strings.TrimSpace(query)
	switch v {
	case viewBacklog:
		m.backlogQuery = q
		m.refreshView()
		m.backlogSel = 0
		m.clampBacklogSel()
	case viewBoard:
		m.boardQuery = q
		m.reload()
		m.colIdx, m.cardIdx = 0, 0
		m.clampSelection()
	}
	m.search = nil
	m.mode = modeNormal
	return m, nil
}

// closeSearch dismisses the overlay, leaving the live filter as it was.
func (m *Model) closeSearch() {
	m.search = nil
	m.mode = modeNormal
}

// --- chrome ------------------------------------------------------------------

// renderSearch draws the search overlay: a heading, the magnifier-prompted
// query field, a live match count, and the Apply / Clear actions.
func (m Model) renderSearch() string {
	t := m.theme
	s := m.search
	var b strings.Builder

	mag := lipgloss.NewStyle().Foreground(t.P.FocusGlow).Render(iconSearch)
	b.WriteString(mag + "  " + s.input.View() + "\n\n")

	// Feedback: an invitation before there's a query, then a live count — tinted
	// danger when nothing matches so a dead end reads at a glance.
	if s.draft() == "" {
		b.WriteString(t.HelpDesc.Render("Type to filter — results update as you go."))
	} else {
		n := s.matchCount()
		style := t.StatusText
		if n == 0 {
			style = t.Error
		}
		b.WriteString(style.Render(matchSummary(n)))
	}
	b.WriteString("\n\n\n")
	b.WriteString(buttonRow(t, []string{"Apply", "Clear"}, s.buttonFocus()))

	return modalShell(t, modalWidthList, 0,
		"Search "+searchScope(s.target),
		"Show only the issues matching every term.",
		b.String(),
		modalHint(t, "enter apply · tab next · esc cancel"))
}

// filterHeader is the section heading a filtered view wears in place of its
// ordinary one: the scope leads (BOARD / BACKLOG, in the same uppercase tone as
// every section title), then the live query and an "N of M" count, with the key
// to lift the filter trailing on the right. Folding the filter into the heading —
// rather than floating a separate bar above the list — keeps each view to one
// header row, so toggling a search never shifts the content beneath it.
func filterHeader(t theme.Theme, scope, query string, matched, total, width int) string {
	sep := t.HelpDesc.Render(" · ")
	mag := lipgloss.NewStyle().Foreground(t.P.Focus).Render(iconSearch)
	q := lipgloss.NewStyle().Foreground(t.P.TextPrimary).Render(`"` + truncate(query, max(8, width/3)) + `"`)
	count := t.HelpDesc.Render(fmt.Sprintf("%d of %d", matched, total))
	left := "  " + sectionTitle(t, scope) + sep + mag + " " + q + sep + count
	right := t.HelpDesc.Render("esc clears") + "  "
	return spread(left, right, width)
}

// searchEmpty is the body shown when a filter matches nothing: a quiet dead-end
// that names the query and the two ways out.
func (m Model) searchEmpty(query string) string {
	t := m.theme
	return lipgloss.JoinVertical(lipgloss.Center,
		t.Title.Render("No matches"),
		"",
		t.HelpDesc.Render(fmt.Sprintf("Nothing here matches %q.", query)),
		t.HelpDesc.Render("Press esc to clear, or ⌘F to search again."),
	)
}
