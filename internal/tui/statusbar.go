package tui

import (
	"image/color"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

// renderHeader is the top bar: the Jeera mark and active project on the left,
// the live MCP "wire" pill on the right.
func (m Model) renderHeader() string {
	t := m.theme
	brand := lipgloss.NewStyle().
		Foreground(t.P.BgBase).Background(t.P.Focus).Bold(true).
		Padding(0, 1).Render("Jeera")

	right := m.mcpPill()
	if badge := m.runBadge(); badge != "" {
		right = badge + "   " + right
	}
	proj := t.HelpDesc.Render("no project — press n to create one")
	if m.active.ID != 0 {
		// Truncate the project name so a long one never overflows the bar.
		budget := m.width - 4 - lipgloss.Width(brand) - lipgloss.Width(right) - lipgloss.Width(m.active.KeyPrefix) - 2
		if budget < 6 {
			budget = 6
		}
		proj = t.Title.Render(truncate(m.active.Name, budget)) + " " + t.CardMeta.Render(m.active.KeyPrefix)
	}
	left := brand + " " + proj

	inner := spread(left, right, m.width-2)
	return lipgloss.NewStyle().Background(t.P.BgSurface).Width(m.width).Render(" " + inner + " ")
}

// renderNavbar is the destination strip beneath the header: Board · Sprints ·
// Runs. The active destination carries the iris accent and a heavy underline;
// the inactive ones sit muted on a light hairline. The in-ticket tab strip is
// intended to adopt this same treatment, so the two read as one system.
func (m Model) renderNavbar() string {
	t := m.theme
	items := []struct {
		v     view
		label string
	}{
		{viewBoard, "Board"},
		{viewSprints, "Sprints"},
		{viewRuns, "Runs"},
	}

	labels := make([]string, len(items))
	// The underline starts under the row's single leading space, then tracks each
	// tab's full width so the accent sits squarely beneath the active label.
	rule := lipgloss.NewStyle().Foreground(t.P.Border).Render("─")
	for i, it := range items {
		if it.v == m.view {
			labels[i] = t.TabActive.Render(it.label)
			rule += lipgloss.NewStyle().Foreground(t.P.Focus).Render(strings.Repeat("━", lipgloss.Width(labels[i])))
		} else {
			labels[i] = t.Tab.Render(it.label)
			rule += lipgloss.NewStyle().Foreground(t.P.Border).Render(strings.Repeat("─", lipgloss.Width(labels[i])))
		}
	}
	tabs := " " + lipgloss.JoinHorizontal(lipgloss.Top, labels...)

	band := lipgloss.NewStyle().Background(t.P.BgSurface).Width(m.width)
	return lipgloss.JoinVertical(lipgloss.Left, band.Render(tabs), band.Render(rule))
}

// renderFooter is the bottom status bar: key hints on the left, transient
// toast/error on the right. The hints are budgeted to the available width so
// the bar never wraps; help and quit are always kept.
func (m Model) renderFooter() string {
	t := m.theme
	right := ""
	switch {
	case m.errText != "":
		right = t.Error.Render("! " + truncate(m.errText, m.width/3))
	case m.toastText != "":
		right = t.Toast.Render(m.toastText)
	}
	inner := spread(m.shortHelp(m.width-2-lipgloss.Width(right)-1), right, m.width-2)
	return lipgloss.NewStyle().Background(t.P.BgSurface).Width(m.width).Render(" " + inner + " ")
}

// shortHelp renders a one-line key hint strip that fits within budget. The help
// and quit hints are always reserved at the end; the navigation hints fill
// whatever space remains.
func (m Model) shortHelp(budget int) string {
	t := m.theme
	sep := t.HelpDesc.Render("  ")
	seg := func(b key.Binding) string {
		h := b.Help()
		return t.HelpKey.Render(h.Key) + " " + t.HelpDesc.Render(h.Desc)
	}

	tail := seg(m.keys.Help) + sep + seg(m.keys.Quit)
	navBudget := budget - lipgloss.Width(tail) - lipgloss.Width(sep)

	var nav strings.Builder
	w := 0
	for _, b := range m.navHints() {
		s := seg(b)
		add := lipgloss.Width(s)
		if nav.Len() > 0 {
			add += lipgloss.Width(sep)
		}
		if w+add > navBudget {
			break
		}
		if nav.Len() > 0 {
			nav.WriteString(sep)
		}
		nav.WriteString(s)
		w += add
	}
	if nav.Len() == 0 {
		return tail
	}
	return nav.String() + sep + tail
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
