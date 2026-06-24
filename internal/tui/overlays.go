package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// --- help --------------------------------------------------------------------

func (m Model) renderHelp() string {
	t := m.theme
	groups := m.keys.FullHelp()
	cols := make([]string, 0, len(groups))
	for _, group := range groups {
		lines := make([]string, 0, len(group))
		for _, b := range group {
			h := b.Help()
			lines = append(lines, t.HelpKey.Width(9).Render(h.Key)+t.HelpDesc.Render(h.Desc))
		}
		col := lipgloss.JoinVertical(lipgloss.Left, lines...)
		cols = append(cols, lipgloss.NewStyle().MarginRight(3).Render(col))
	}
	body := t.Title.Render("Keys") + "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	body += "\n\n" + t.HelpDesc.Render("press any key to close")
	return t.Modal.Render(body)
}

// --- MCP ---------------------------------------------------------------------

func (m Model) renderMCP() string {
	t := m.theme
	var b strings.Builder
	b.WriteString(t.Title.Render("MCP server"))
	b.WriteString("\n\n")
	if m.mcp == nil {
		b.WriteString(t.HelpDesc.Render("Started with --no-mcp; the server is off."))
		b.WriteString("\n\n" + t.HelpDesc.Render("press any key to close"))
		return t.Modal.Render(b.String())
	}
	st := m.mcp.Status()
	status := "starting"
	switch {
	case st.Err != nil:
		status = "error: " + st.Err.Error()
	case st.Listening:
		status = "listening"
	}
	b.WriteString(t.Label.Render("Status") + "  " + t.StatusText.Render(status) + "\n")
	b.WriteString(t.Label.Render("URL") + "     " + t.CardKey.Render(st.URL) + "\n\n")
	b.WriteString(t.Label.Render("Connect with Claude Code:") + "\n")
	b.WriteString(t.StatusText.Render("claude mcp add --transport http jeera "+st.URL) + "\n\n")
	b.WriteString(t.Label.Render("Or add to .mcp.json:") + "\n")
	b.WriteString(t.StatusText.Render(m.mcp.ClientConfigJSON()))
	b.WriteString("\n\n" + t.HelpDesc.Render("press any key to close"))
	return t.Modal.Render(b.String())
}

// --- projects ----------------------------------------------------------------

func (m Model) renderProjects() string {
	t := m.theme
	var b strings.Builder
	b.WriteString(t.Title.Render("Projects"))
	b.WriteString("\n\n")
	if len(m.projects) == 0 {
		b.WriteString(t.HelpDesc.Render("No projects yet."))
	}
	for i, p := range m.projects {
		cursor := "  "
		nameStyle := t.StatusText
		if i == m.projSel {
			cursor = t.HelpKey.Render("▸ ")
			nameStyle = t.CardTitle
		}
		active := ""
		if p.ID == m.active.ID {
			active = t.Chip.Render("  • active")
		}
		b.WriteString(cursor + nameStyle.Render(p.Name) + " " + t.CardMeta.Render(p.KeyPrefix) + active + "\n")
	}
	b.WriteString("\n" + t.HelpDesc.Render("↑/↓ select · enter open · n new · esc close"))
	return t.Modal.Width(48).Render(b.String())
}

func (m Model) updateProjects(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.mode = modeBoard
	case "up", "k":
		if m.projSel > 0 {
			m.projSel--
		}
	case "down", "j":
		if m.projSel < len(m.projects)-1 {
			m.projSel++
		}
	case "n":
		m.form = newCreateProjectForm()
		m.mode = modeForm
		return m, m.form.focusCmd()
	case "enter":
		if m.projSel >= 0 && m.projSel < len(m.projects) {
			m.active = m.projects[m.projSel]
			m.colIdx, m.cardIdx = 0, 0
			m.mode = modeBoard
			m.reload()
			return m, toast("switched to " + m.active.KeyPrefix)
		}
	}
	return m, nil
}

// --- confirm -----------------------------------------------------------------

func (m Model) renderConfirm() string {
	t := m.theme
	body := t.Title.Render("Confirm") + "\n\n" +
		t.StatusText.Render(m.confirm) + "\n\n" +
		t.HelpKey.Render("y") + " " + t.HelpDesc.Render("yes") + "   " +
		t.HelpKey.Render("n") + " " + t.HelpDesc.Render("no")
	return t.Modal.Render(body)
}

func (m Model) updateConfirm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		var cmd tea.Cmd
		if m.onConfirm != nil {
			cmd = m.onConfirm()
		}
		m.mode = modeBoard
		m.onConfirm = nil
		m.reload()
		return m, cmd
	case "n", "N", "esc", "q":
		m.mode = modeBoard
		m.onConfirm = nil
	}
	return m, nil
}
