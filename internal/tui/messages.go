package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// storeEventMsg carries a store change event into the Bubble Tea update loop.
// The event bridge (see Run) forwards every store event as one of these via
// Program.Send, so the board refreshes the instant an agent writes over MCP.
type storeEventMsg struct{ ev core.Event }

// errMsg surfaces a non-fatal error to the status bar.
type errMsg struct{ err error }

// toastMsg shows a transient confirmation message.
type toastMsg struct{ text string }

// clearToastMsg clears the current toast.
type clearToastMsg struct{}

// discussFinishedMsg is delivered when an interactive "Discuss" session (run via
// tea.ExecProcess) exits and the TUI resumes; the ticket may have changed.
type discussFinishedMsg struct{ err error }

// toast returns a command that shows text now and clears it after a short delay.
func toast(text string) tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return toastMsg{text} },
		tea.Tick(3*time.Second, func(time.Time) tea.Msg { return clearToastMsg{} }),
	)
}

func reportErr(err error) tea.Cmd {
	return func() tea.Msg { return errMsg{err} }
}
