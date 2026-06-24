package tui

import (
	"fmt"

	"charm.land/lipgloss/v2"
)

// renderRuns is the global Runs view: recent executions across all tickets, with
// their version, provider, status and timing. It refreshes live as runs change.
func (m Model) renderRuns(height int) string {
	t := m.theme
	title := t.Title.Render("Runs") + "   " + t.HelpDesc.Render(fmt.Sprintf("%d active", m.activeRuns))
	lines := []string{title, ""}

	if len(m.recentRuns) == 0 {
		lines = append(lines, t.HelpDesc.Render("No runs yet. Open a ticket and press s to Start one."))
	}
	for _, r := range m.recentRuns {
		key := "?"
		if iss, err := m.store.GetIssue(r.IssueID); err == nil {
			key = iss.Key
		}
		statusStyle := lipgloss.NewStyle().Foreground(t.RunStateColor(r.Status))
		assignee := string(r.Provider)
		if r.Model != "" {
			assignee += "·" + r.Model
		}
		when := ""
		if r.StartedAt != nil {
			when = r.StartedAt.Local().Format("15:04:05")
		}
		line := t.CardKey.Render(fmt.Sprintf("%-9s", fmt.Sprintf("%s v%d", key, r.Version))) + "  " +
			statusStyle.Render(fmt.Sprintf("%-10s", r.Status)) + "  " +
			t.Chip.Render(assignee) + "   " +
			t.CardMeta.Render(when)
		lines = append(lines, line)
	}

	body := fitHeight(lipgloss.JoinVertical(lipgloss.Left, lines...), height-1)
	hint := t.HelpDesc.Render("press any key to close")
	return lipgloss.JoinVertical(lipgloss.Left, body, hint)
}

// runBadge is the active-run indicator for the header (empty when none).
func (m Model) runBadge() string {
	if m.activeRuns <= 0 {
		return ""
	}
	t := m.theme
	dot := lipgloss.NewStyle().Foreground(t.P.Focus).Render("●")
	return dot + " " + t.StatusText.Render(fmt.Sprintf("%d running", m.activeRuns))
}
