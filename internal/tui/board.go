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

const columnGutter = 1

// renderBoard draws the kanban columns (or an empty state) in a region exactly
// `height` rows tall, so the footer stays pinned to the bottom.
func (m Model) renderBoard(height int) string {
	t := m.theme
	if m.active.ID == 0 {
		return m.center(m.welcome(), height)
	}
	if len(m.board.columns) == 0 {
		return m.center(t.Empty.Render("This project has no columns."), height)
	}

	n := len(m.board.columns)
	colW := (m.width - (n-1)*columnGutter) / n
	if colW < 16 {
		colW = 16
	}

	blocks := make([]string, 0, n*2)
	for i, col := range m.board.columns {
		if i > 0 {
			blocks = append(blocks, fitHeight(" ", height)) // gutter column
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
	hint := t.HelpKey.Render("n") + " " + t.HelpDesc.Render("create your first project") +
		"   " + t.HelpKey.Render("?") + " " + t.HelpDesc.Render("help")
	return lipgloss.JoinVertical(lipgloss.Center, title, "", body, "", hint)
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
	if len(col.cards) == 0 {
		lines = append(lines, t.Empty.Width(colW).Render(t.HelpDesc.Render("— empty —")))
	}
	for ci, iss := range col.cards {
		selected := idx == m.colIdx && ci == m.cardIdx
		lines = append(lines, m.renderCard(iss, selected, colW))
	}
	col2 := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return fitHeight(col2, height)
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

// updateBoard handles keys while the board is focused.
func (m Model) updateBoard(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.mode = modeHelp
	case key.Matches(msg, m.keys.MCP):
		m.mode = modeMCP
	case key.Matches(msg, m.keys.Runs):
		m.recentRuns, _ = m.store.ListRecentRuns(50)
		m.mode = modeRuns
	case key.Matches(msg, m.keys.Project):
		m.mode = modeProjects
		m.projSel = m.activeProjectIndex()
	case key.Matches(msg, m.keys.Refresh):
		m.reload()
		return m, toast("refreshed")

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

	case key.Matches(msg, m.keys.New):
		if m.active.ID == 0 {
			m.form = newCreateProjectForm()
		} else {
			m.form = newCreateIssueForm()
		}
		m.mode = modeForm
		return m, m.form.focusCmd()
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
	case key.Matches(msg, m.keys.MoveRight):
		return m.moveSelected(+1)
	case key.Matches(msg, m.keys.MoveLeft):
		return m.moveSelected(-1)
	case key.Matches(msg, m.keys.Enter):
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
