package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// spread lays out left and right on one line of the given width, filling the
// gap between them with spaces. If the two together exceed the width, they are
// separated by a single space and allowed to overflow.
func spread(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// truncate shortens s to at most width display cells, adding an ellipsis when
// it cuts. It operates on runes, so it is safe for multi-byte text.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	if width == 1 {
		return "…"
	}
	out := make([]rune, 0, width)
	w := 0
	for _, r := range runes {
		rw := lipgloss.Width(string(r))
		if w+rw > width-1 {
			break
		}
		out = append(out, r)
		w += rw
	}
	return string(out) + "…"
}
