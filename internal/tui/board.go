package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

const columnGutter = 3

// renderBoard draws the kanban columns (or an empty state) in a region exactly
// `height` rows tall, so the footer stays pinned to the bottom. While a search
// is live it heads the columns with a filter header — the same one the Backlog
// wears — or shows a dead-end when nothing matches.
func (m Model) renderBoard(height int) string {
	t := m.theme
	if m.active.ID == 0 {
		return m.center(m.welcome(), height)
	}
	if len(m.board.columns) == 0 {
		return m.center(t.Empty.Render("This project has no columns."), height)
	}

	if m.boardQuery != "" {
		matched := countCards(m.board)
		if matched == 0 {
			return m.center(m.searchEmpty(m.boardQuery), height)
		}
		header := filterHeader(t, "Board", m.boardQuery, matched, m.boardTotal, m.width)
		return fitHeight(lipgloss.JoinVertical(lipgloss.Left, header, "", m.renderColumns(max(0, height-2))), height)
	}
	return m.renderColumns(height)
}

// renderColumns lays the kanban lanes side by side in a region exactly `height`
// rows tall.
func (m Model) renderColumns(height int) string {
	n := len(m.board.columns)
	colW := (m.width - (n-1)*columnGutter) / n
	if colW < 16 {
		colW = 16
	}

	blocks := make([]string, 0, n*2)
	for i, col := range m.board.columns {
		if i > 0 {
			blocks = append(blocks, fitHeight(strings.Repeat(" ", columnGutter), height)) // gutter column
		}
		blocks = append(blocks, m.renderColumn(col, i, colW, height))
	}
	board := lipgloss.JoinHorizontal(lipgloss.Top, blocks...)
	return fitHeight(board, height)
}

func (m Model) welcome() string {
	t := m.theme
	title := t.Title.Render("Welcome to Jeera")
	body := t.HelpDesc.Render("An agentic-first issue tracker for your terminal.")
	cta := button(t, iconAdd+" Create your first project", true)
	hint := t.HelpDesc.Render("press enter to begin · ? for help")
	return lipgloss.JoinVertical(lipgloss.Center, title, "", body, "", cta, "", hint)
}

func (m Model) renderColumn(col column, idx, colW, height int) string {
	t := m.theme
	cat := t.CategoryColor(col.status.Category)

	dot := lipgloss.NewStyle().Foreground(cat).Render("●")
	name := t.ColumnTitle.Foreground(cat).Render(truncate(col.status.Name, colW-6))
	count := t.CountBadge.Render(fmt.Sprintf("%d", len(col.cards)))
	header := spread(dot+" "+name, count, colW)
	rule := lipgloss.NewStyle().Foreground(t.P.Border).Render(strings.Repeat("─", colW))

	lines := []string{header, rule}
	for ci, iss := range col.cards {
		selected := idx == m.colIdx && ci == m.cardIdx
		lines = append(lines, m.renderCard(iss, selected, colW))
	}
	// The "+ New issue" affordance is the slot one past the last card on the To Do
	// column — new work enters the board there, so the downstream lanes stay free
	// of a create action that never made sense on them.
	if m.columnHasAddCard(idx) {
		addSel := idx == m.colIdx && m.cardIdx == len(col.cards)
		lines = append(lines, m.renderAddCard(colW, addSel))
	}
	col2 := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return fitHeight(col2, height)
}

// ghostBorder is a dashed rounded frame — the "empty slot" look for the
// add-issue affordance, distinct from a real card's solid border.
var ghostBorder = lipgloss.Border{
	Top: "╌", Bottom: "╌", Left: "╎", Right: "╎",
	TopLeft: "╭", TopRight: "╮", BottomLeft: "╰", BottomRight: "╯",
}

// renderAddCard draws a column's "+ New issue" button. Selected, it glows iris
// like any focused control; otherwise it sits as a quiet dashed placeholder.
func (m Model) renderAddCard(colW int, selected bool) string {
	t := m.theme
	bc, fc := t.P.Border, t.P.TextSubtle
	if selected {
		bc, fc = t.P.FocusGlow, t.P.FocusGlow
	}
	return lipgloss.NewStyle().
		Border(ghostBorder).BorderForeground(bc).
		Foreground(fc).Bold(selected).
		Width(colW).Align(lipgloss.Center).Padding(0, 1).
		Render(iconAdd + " New issue")
}

// columnHasAddCard reports whether a column shows the "+ New issue" affordance.
// Only To Do (todo-category) columns do: new work enters the board there, so the
// downstream lanes never carry a create action that didn't belong on them. It is
// hidden while a search is live — a filtered board is for finding, not creating,
// and a fresh issue would likely fall outside the filter and vanish.
func (m Model) columnHasAddCard(idx int) bool {
	return m.boardQuery == "" && idx >= 0 && idx < len(m.board.columns) &&
		m.board.columns[idx].status.Category == core.CategoryTodo
}

// onAddCard reports whether the board cursor is on a column's "+ New issue" slot
// rather than a real card.
func (m Model) onAddCard() bool {
	if !m.columnHasAddCard(m.colIdx) {
		return false
	}
	return m.cardIdx == len(m.board.columns[m.colIdx].cards)
}

func (m Model) renderCard(iss core.Issue, selected bool, colW int) string {
	t := m.theme
	// Lip Gloss Width is the total box width; the text area is colW minus the
	// rounded border (2) and horizontal padding (2).
	textW := colW - 4
	if textW < 6 {
		textW = 6
	}

	pri := lipgloss.NewStyle().Foreground(t.PriorityColor(iss.Priority)).Render(theme.PriorityGlyph(iss.Priority))
	keyLine := pri + " " + t.CardKey.Render(iss.Key)
	title := t.CardTitle.Render(truncate(iss.Title, textW))

	var metas []string
	if !iss.Assignee.IsZero() {
		chip := "◇ " + iss.Assignee.Model
		if iss.Assignee.Effort != "" {
			chip += "·" + string(iss.Assignee.Effort)
		}
		metas = append(metas, t.Chip.Render(chip))
	}
	if iss.StoryPoints != nil {
		metas = append(metas, t.CardMeta.Render(fmt.Sprintf("%dpt", *iss.StoryPoints)))
	}
	if tags := m.board.tags[iss.ID]; len(tags) > 0 {
		metas = append(metas, t.Tag.Render("#"+strings.Join(tags, " #")))
	}

	body := keyLine + "\n" + title
	if len(metas) > 0 {
		body += "\n" + truncate(strings.Join(metas, "  "), textW)
	}

	style := t.Card
	if selected {
		style = t.CardSelected
	}
	return style.Width(colW).Render(body)
}

// handleGlobalKey handles the keys that work the same on every view: switching
// destinations and opening the overlays. It returns handled=false for anything
// the active view should interpret itself (its own navigation and actions).
func (m Model) handleGlobalKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit, true
	case key.Matches(msg, m.keys.NextView):
		m.switchView(+1)
		return m, nil, true
	case key.Matches(msg, m.keys.PrevView):
		m.switchView(-1)
		return m, nil, true
	case key.Matches(msg, m.keys.Help):
		m.mode = modeHelp
		return m, nil, true
	case key.Matches(msg, m.keys.MCP):
		m.mode = modeMCP
		return m, nil, true
	case key.Matches(msg, m.keys.Settings):
		m.settings = newSettings(m.cfg, m.theme, m.width, m.height)
		m.mode = modeSettings
		return m, nil, true
	case key.Matches(msg, m.keys.Project):
		m.mode = modeProjects
		m.projSel = m.activeProjectIndex()
		return m, nil, true
	case key.Matches(msg, m.keys.Refresh):
		m.reload()
		return m, toast("refreshed"), true
	}
	return m, nil, false
}

// updateBoard handles the board's deliberately small keyset: arrows to move the
// cursor, ⇧+arrows to move the selected ticket across columns, e/x/enter to
// rename/delete/open. Creating is a button (the To Do column's "+ New issue"
// slot), not a keystroke. The global keys are intercepted earlier by handleGlobalKey.
func (m Model) updateBoard(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Welcome screen (no project yet): the only move is creating the first
	// project, offered as the focused button — enter (or n) starts it.
	if m.active.ID == 0 {
		if key.Matches(msg, m.keys.Enter) || key.Matches(msg, m.keys.New) {
			m.form = newCreateProjectForm()
			m.mode = modeForm
			return m, m.form.focusCmd()
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Search):
		return m.openSearch()
	case msg.String() == "esc" && m.boardQuery != "":
		return m.applySearch(viewBoard, "") // esc lifts a live filter
	case key.Matches(msg, m.keys.Up):
		m.cardIdx--
		m.clampSelection()
	case key.Matches(msg, m.keys.Down):
		m.cardIdx++
		m.clampSelection()
	case key.Matches(msg, m.keys.Left):
		m.colIdx--
		m.clampSelection()
	case key.Matches(msg, m.keys.Right):
		m.colIdx++
		m.clampSelection()
	case key.Matches(msg, m.keys.MoveRight):
		return m.moveSelected(+1)
	case key.Matches(msg, m.keys.MoveLeft):
		return m.moveSelected(-1)

	case key.Matches(msg, m.keys.Edit):
		if iss, ok := m.selectedIssue(); ok {
			m.form = newRenameForm(iss)
			m.mode = modeForm
			return m, m.form.focusCmd()
		}
	case key.Matches(msg, m.keys.Delete):
		if iss, ok := m.selectedIssue(); ok {
			id := iss.ID
			m.confirm = fmt.Sprintf("Delete %s — %s?", iss.Key, truncate(iss.Title, 40))
			sched := m.sched
			m.onConfirm = func() tea.Cmd {
				// Stop any live cron jobs first — the schedule rows cascade away with
				// the issue, but the in-memory jobs would otherwise keep firing.
				if sched != nil {
					sched.RemoveForIssue(id)
				}
				if err := m.store.DeleteIssue(id); err != nil {
					return reportErr(err)
				}
				return toast("deleted")
			}
			m.mode = modeConfirm
		}
	case key.Matches(msg, m.keys.Enter):
		if m.onAddCard() {
			// Create directly into this column's status.
			m.form = newCreateIssueForm(m.board.columns[m.colIdx].status.ID)
			m.mode = modeForm
			return m, m.form.focusCmd()
		}
		if iss, ok := m.selectedIssue(); ok {
			m.detail = newDetail(m.store, m.runMgr, m.sched, m.theme, iss.ID, m.width, m.height)
			m.mode = modeDetail
		}
	}
	return m, nil
}

// moveSelected transitions the selected issue to the adjacent column and keeps
// the selection on it.
func (m Model) moveSelected(dir int) (tea.Model, tea.Cmd) {
	iss, ok := m.selectedIssue()
	if !ok {
		return m, nil
	}
	target := m.colIdx + dir
	if target < 0 || target >= len(m.board.columns) {
		return m, nil
	}
	dest := m.board.columns[target].status
	if err := m.store.TransitionIssue(iss.ID, dest.ID); err != nil {
		return m, reportErr(err)
	}
	m.reload()
	m.selectIssueByID(iss.ID)
	return m, nil
}

func (m *Model) selectIssueByID(id int64) {
	for ci, col := range m.board.columns {
		for ri, card := range col.cards {
			if card.ID == id {
				m.colIdx, m.cardIdx = ci, ri
				return
			}
		}
	}
	m.clampSelection()
}

func (m Model) activeProjectIndex() int {
	for i, p := range m.projects {
		if p.ID == m.active.ID {
			return i
		}
	}
	return 0
}

// fitHeight pads with blank lines or clips so s is exactly height rows.
func fitHeight(s string, height int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
