package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
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
	body := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	return modalShell(t, modalWidthHelp, 0, "Keys",
		"⌥tab switches views · tab moves focus inside a view · ⇧+arrows move a ticket",
		body, modalHint(t, "press any key to close"))
}

// --- MCP ---------------------------------------------------------------------

func (m Model) renderMCP() string {
	t := m.theme
	const sub = "The agent endpoint Jeera serves while the board is open."
	if m.mcp == nil {
		return modalShell(t, modalWidthMCP, 3, "MCP server", sub,
			t.HelpDesc.Render("Started with --no-mcp; the server is off."),
			modalHint(t, "press any key to close"))
	}
	st := m.mcp.Status()
	status := "starting"
	switch {
	case st.Err != nil:
		status = "error: " + st.Err.Error()
	case st.Listening:
		status = "listening"
	}
	var b strings.Builder
	b.WriteString(t.Label.Render(fmt.Sprintf("%-8s", "Status")) + t.StatusText.Render(status) + "\n")
	b.WriteString(t.Label.Render(fmt.Sprintf("%-8s", "URL")) + t.CardKey.Render(st.URL) + "\n\n")
	b.WriteString(t.Label.Render("Connect with Claude Code") + "\n")
	b.WriteString(t.StatusText.Render("claude mcp add --transport http jeera "+st.URL) + "\n\n")
	b.WriteString(t.Label.Render("Or add to .mcp.json") + "\n")
	b.WriteString(t.StatusText.Render(m.mcp.ClientConfigJSON()))
	return modalShell(t, modalWidthMCP, 0, "MCP server", sub, b.String(), modalHint(t, "press any key to close"))
}

// --- projects ----------------------------------------------------------------

func (m Model) renderProjects() string {
	t := m.theme
	const inner = modalWidthList - 6
	if len(m.projects) == 0 {
		return modalShell(t, modalWidthList, 3, "Projects", "",
			t.HelpDesc.Render("No projects yet — press n to add one."),
			modalHint(t, "n new · esc close"))
	}
	defaultPrefix := ""
	if m.cfg != nil {
		defaultPrefix = m.cfg.Get().DefaultProjectPrefix
	}
	rows := make([]string, 0, len(m.projects))
	for i, p := range m.projects {
		cursor := "  "
		nameStyle := t.StatusText
		if i == m.projSel {
			cursor = t.HelpKey.Render("▸ ")
			nameStyle = t.CardTitle
		}
		// Two quiet chips from the same family: where you are now (active) and
		// where Jeera opens (default). A project that is both wears both.
		chips := ""
		if p.ID == m.active.ID {
			chips += t.Chip.Render("  • active")
		}
		if defaultPrefix != "" && p.KeyPrefix == defaultPrefix {
			chips += t.Chip.Render("  ★ default")
		}
		row := cursor + nameStyle.Render(p.Name) + " " + t.CardMeta.Render(p.KeyPrefix) + chips
		if p.RepoPath != "" {
			row += "\n    " + t.HelpDesc.Render(truncate(p.RepoPath, inner-4))
		}
		rows = append(rows, row)
	}
	// Two rows of hints, grouped by intent: getting around on top, acting on the
	// selected project below — too many verbs for one line at this width.
	hint := modalHint(t, "↑/↓ select · enter open · esc close") + "\n" +
		modalHint(t, "n new · e edit · x delete · d default")
	return modalShell(t, modalWidthList, 5, "Projects", "",
		strings.Join(rows, "\n"), hint)
}

func (m Model) updateProjects(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.mode = modeNormal
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
	case "e":
		if m.projSel >= 0 && m.projSel < len(m.projects) {
			m.form = newEditProjectForm(m.projects[m.projSel])
			m.mode = modeForm
			return m, m.form.focusCmd()
		}
	case "x":
		if m.projSel >= 0 && m.projSel < len(m.projects) {
			return m.confirmDeleteProject(m.projects[m.projSel])
		}
	case "d":
		if m.cfg != nil && m.projSel >= 0 && m.projSel < len(m.projects) {
			p := m.projects[m.projSel]
			if err := m.cfg.SetDefaultProject(p.KeyPrefix); err != nil {
				return m, reportErr(err)
			}
			return m, toast(p.KeyPrefix + " opens on startup")
		}
	case "enter":
		if m.projSel >= 0 && m.projSel < len(m.projects) {
			m.active = m.projects[m.projSel]
			m.colIdx, m.cardIdx = 0, 0
			m.backlogSel, m.sprintSel = 0, 0 // a new project has its own issues; don't carry old cursors
			m.mode = modeNormal
			m.reload()
			return m, toast("switched to " + m.active.KeyPrefix)
		}
	}
	return m, nil
}

// confirmDeleteProject asks before removing a project. Unlike a sprint — whose
// issues only fall back to the backlog — deleting a project cascades: its issues,
// sprints, runs and schedules go with it, for good. So the prompt is explicit;
// the live cron jobs are torn down before their rows vanish; and a default pin on
// this project is cleared so a later project can't silently inherit it by prefix.
func (m Model) confirmDeleteProject(p core.Project) (tea.Model, tea.Cmd) {
	m.confirm = fmt.Sprintf("Delete project %q (%s)? This permanently deletes all its issues, sprints and runs.", p.Name, p.KeyPrefix)
	id, prefix := p.ID, p.KeyPrefix
	st, sched, cfg := m.store, m.sched, m.cfg
	m.onConfirm = func() tea.Cmd {
		if sched != nil {
			sched.RemoveForProject(id)
		}
		if err := st.DeleteProject(id); err != nil {
			return reportErr(err)
		}
		if cfg != nil && cfg.Get().DefaultProjectPrefix == prefix {
			_ = cfg.SetDefaultProject("")
		}
		return toast("deleted project " + prefix)
	}
	m.mode = modeConfirm
	return m, nil
}

// --- confirm -----------------------------------------------------------------

func (m Model) renderConfirm() string {
	t := m.theme
	body := t.StatusText.Render(m.confirm)
	hint := t.HelpKey.Render("y") + " " + t.HelpDesc.Render("yes") + "   " +
		t.HelpKey.Render("n") + " " + t.HelpDesc.Render("no")
	return modalShell(t, modalWidthConfirm, 2, "Confirm", "", body, hint)
}

func (m Model) updateConfirm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		var cmd tea.Cmd
		if m.onConfirm != nil {
			cmd = m.onConfirm()
		}
		m.mode = modeNormal
		m.onConfirm = nil
		m.reload()
		return m, cmd
	case "n", "N", "esc", "q":
		m.mode = modeNormal
		m.onConfirm = nil
	}
	return m, nil
}
