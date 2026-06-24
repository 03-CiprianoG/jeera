package tui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

// tabStrip renders a horizontal strip of labels on a BgSurface band: the active
// one carries the iris accent and a heavy underline, the rest sit muted on a
// light hairline. The top navbar and the ticket detail's tab row both render
// through here, so the two read as one system — learn one, know both.
func tabStrip(t theme.Theme, width int, labels []string, active int) string {
	rendered := make([]string, len(labels))
	// The underline starts under the row's single leading space, then tracks each
	// label's full width so the accent sits squarely beneath the active one.
	rule := lipgloss.NewStyle().Foreground(t.P.Border).Render("─")
	for i, label := range labels {
		if i == active {
			rendered[i] = t.TabActive.Render(label)
			rule += lipgloss.NewStyle().Foreground(t.P.Focus).Render(strings.Repeat("━", lipgloss.Width(rendered[i])))
		} else {
			rendered[i] = t.Tab.Render(label)
			rule += lipgloss.NewStyle().Foreground(t.P.Border).Render(strings.Repeat("─", lipgloss.Width(rendered[i])))
		}
	}
	tabs := " " + lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
	band := lipgloss.NewStyle().Background(t.P.BgSurface).Width(width)
	return lipgloss.JoinVertical(lipgloss.Left, band.Render(tabs), band.Render(rule))
}
