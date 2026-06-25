package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// renderNavbar is the app's primary wayfinding: a big, centered strip of
// icon+label pills, the active one filled iris. It walks with ⌥tab so plain tab
// stays free for moving focus inside a view.
func (m Model) renderNavbar() string {
	items := []navItem{
		{iconBoard, "Board"},
		{iconBacklog, "Backlog"},
		{iconSprints, "Sprints"},
		{iconRuns, "Runs"},
	}
	return navbar(m.theme, m.width, items, int(m.view), brandLogo(m.theme))
}

// renderFooter is the bottom chrome — the active project sits on the left, the
// live MCP "wire" and a single ? help affordance on the right, with any
// transient toast or error riding just ahead of them. The Jeera wordmark now
// leads the header up top, so the footer no longer carries the brand.
func (m Model) renderFooter() string {
	t := m.theme
	help := t.HelpKey.Render(iconHelp) + " " + t.HelpDesc.Render("help")

	// The right cluster is the always-on identity: the live wire and the one help
	// affordance (plus the run badge when something is running). It is never
	// dropped, so the project name on the left yields space to it first.
	core := m.mcpPill() + "   " + help
	if badge := m.runBadge(); badge != "" {
		core = badge + "   " + core
	}

	proj := t.HelpDesc.Render("no project yet")
	if m.active.ID != 0 {
		budget := m.width - 4 - lipgloss.Width(core) - 1 - lipgloss.Width(m.active.KeyPrefix)
		if budget < 4 {
			budget = 4
		}
		proj = t.StatusText.Render(truncate(m.active.Name, budget)) + " " + t.CardMeta.Render(m.active.KeyPrefix)
	}
	left := proj

	// Transient feedback rides just ahead of the core cluster — but only when it
	// fits the gap, so a long error at a narrow width can never wrap the bar.
	right := core
	feedback := ""
	switch {
	case m.errText != "":
		feedback = t.Error.Render("! " + m.errText)
	case m.toastText != "":
		feedback = t.Toast.Render(m.toastText)
	}
	if feedback != "" {
		gap := (m.width - 2) - lipgloss.Width(left) - lipgloss.Width(core)
		if avail := gap - 3; avail >= 6 {
			right = ansiClip(feedback, avail) + "   " + core
		}
	}

	inner := spread(left, right, m.width-2)
	return lipgloss.NewStyle().Background(t.P.BgSurface).Width(m.width).Render(" " + inner + " ")
}

// mcpPill renders the always-visible MCP connection indicator — Jeera's
// signature human↔agent "wire".
func (m Model) mcpPill() string {
	t := m.theme
	dot := func(c color.Color) string { return lipgloss.NewStyle().Foreground(c).Render("●") }

	if m.mcp == nil {
		return dot(t.P.TextSubtle) + " " + t.CardMeta.Render("mcp off")
	}
	st := m.mcp.Status()
	switch {
	case st.Err != nil:
		return dot(t.P.Danger) + " " + t.Error.Render("mcp error")
	case st.Listening:
		return dot(t.P.Success) + " " + t.StatusText.Render("jeera "+st.Addr)
	default:
		return dot(t.P.Warning) + " " + t.StatusText.Render("mcp starting")
	}
}
