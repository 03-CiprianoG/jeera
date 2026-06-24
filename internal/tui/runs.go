package tui

import (
	"fmt"
	"os/exec"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// updateRuns drives the Runs overlay while it is focused: a cursor over recent
// runs, with t/enter to re-open the selected run's session in a terminal, w to
// follow a run's live log in a terminal, and esc/q (or R again) to close.
// Unknown keys are ignored so the list stays put rather than dismissing on a
// stray press.
func (m Model) updateRuns(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "R":
		m.mode = modeBoard
	case "up", "k":
		if m.runsCursor > 0 {
			m.runsCursor--
		}
	case "down", "j":
		if m.runsCursor < len(m.recentRuns)-1 {
			m.runsCursor++
		}
	case "t", "enter":
		return m, m.resumeSelectedRun()
	case "w":
		return m, m.watchSelectedRun()
	}
	return m, nil
}

// resumeSelectedRun re-opens the highlighted run's agent session in a new
// terminal — a new tmux/zellij/screen window when Jeera runs inside a
// multiplexer, or a fresh GUI terminal on a desktop. The board stays live and
// the session NEVER takes it over. When no terminal can be reached (a bare
// SSH/headless shell), it copies a ready-to-run command to the clipboard so the
// user can open the session themselves.
func (m Model) resumeSelectedRun() tea.Cmd {
	if m.runMgr == nil {
		return reportErr(fmt.Errorf("run manager unavailable"))
	}
	if m.runsCursor < 0 || m.runsCursor >= len(m.recentRuns) {
		return nil
	}
	r := m.recentRuns[m.runsCursor]
	cmd, err := m.runMgr.ResumeCommand(r)
	if err != nil {
		return reportErr(err)
	}
	return openOrCopy(cmd, fmt.Sprintf("resuming %s", shortSession(r.SessionID)))
}

// watchSelectedRun opens the selected run's live log in a new terminal so the
// human can follow an autonomous run as it works. Autonomous runs stream to a
// log rather than an attachable session, so this tails that log — through the
// same open-or-copy path as resume, and likewise never inline.
func (m Model) watchSelectedRun() tea.Cmd {
	if m.runsCursor < 0 || m.runsCursor >= len(m.recentRuns) {
		return nil
	}
	r := m.recentRuns[m.runsCursor]
	if r.LogPath == "" {
		return reportErr(fmt.Errorf("run v%d has no log to watch yet", r.Version))
	}
	cmd := exec.Command("tail", "-n", "+1", "-f", r.LogPath)
	cmd.Dir = filepath.Dir(r.LogPath)
	return openOrCopy(cmd, fmt.Sprintf("watching run v%d", r.Version))
}

// openOrCopy turns a launch outcome into the right TUI command: a toast naming
// the terminal on success, a clipboard copy (OSC52) plus a toast when there is
// no terminal to open, or an error. It is the one place the TUI launches an
// agent session, and it never runs anything inline.
func openOrCopy(inner *exec.Cmd, what string) tea.Cmd {
	switch out := launchInTerminalOrCopy(inner, what); {
	case out.Err != nil:
		return reportErr(out.Err)
	case out.Copy != "":
		return tea.Batch(tea.SetClipboard(out.Copy), toast(out.Msg))
	default:
		return toast(out.Msg)
	}
}

// runsWindow returns the [start, end) slice of a list of n rows to show so that
// the cursor stays visible within visible rows. It scrolls only as far as needed
// to keep the selection on screen.
func runsWindow(cursor, n, visible int) (int, int) {
	if visible < 1 {
		visible = 1
	}
	if n <= visible {
		return 0, n
	}
	start := 0
	if cursor >= visible {
		start = cursor - visible + 1
	}
	end := start + visible
	if end > n {
		end = n
		start = n - visible
	}
	return start, end
}

// shortSession trims a session/thread id to its first segment for display — the
// leading UUID group is plenty to recognise a run without dominating the row.
func shortSession(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// renderRuns is the global Runs view: recent executions across all tickets, with
// their version, provider, status, timing and session id. The selected row can
// be re-opened in an external terminal. It refreshes live as runs change.
func (m Model) renderRuns(height int) string {
	t := m.theme
	title := t.Title.Render("Runs") + "   " + t.HelpDesc.Render(fmt.Sprintf("%d active", m.activeRuns))
	lines := []string{title, ""}

	if len(m.recentRuns) == 0 {
		lines = append(lines, t.HelpDesc.Render("No runs yet. Open a ticket and press s to Start one."))
	}
	// Window the list so the selected row is always on screen: the rows share the
	// height with the title, the blank line and the footer hint.
	start, end := runsWindow(m.runsCursor, len(m.recentRuns), height-3)
	for i := start; i < end; i++ {
		r := m.recentRuns[i]
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
		marker := "  "
		if i == m.runsCursor {
			marker = t.HelpKey.Render("▸ ")
		}
		session := ""
		if r.SessionID != "" {
			session = "   " + t.HelpDesc.Render("↻ "+shortSession(r.SessionID))
		}
		line := marker +
			t.CardKey.Render(fmt.Sprintf("%-9s", fmt.Sprintf("%s v%d", key, r.Version))) + "  " +
			statusStyle.Render(fmt.Sprintf("%-10s", r.Status)) + "  " +
			t.Chip.Render(assignee) + "   " +
			t.CardMeta.Render(when) +
			session
		lines = append(lines, line)
	}

	body := fitHeight(lipgloss.JoinVertical(lipgloss.Left, lines...), height-1)
	return lipgloss.JoinVertical(lipgloss.Left, body, m.runsHint())
}

// runsHint is the Runs overlay's footer: the resume affordance when there is a
// run to act on, otherwise just how to close.
func (m Model) runsHint() string {
	t := m.theme
	if len(m.recentRuns) == 0 {
		return t.HelpKey.Render("esc") + " " + t.HelpDesc.Render("close")
	}
	seg := func(k, v string) string { return t.HelpKey.Render(k) + " " + t.HelpDesc.Render(v) }
	return seg("↑/↓", "select") + "   " +
		seg("t", "resume") + "   " +
		seg("w", "watch") + "   " +
		seg("esc", "close")
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
