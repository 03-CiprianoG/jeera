package tui

import (
	"strings"

	"charm.land/glamour/v2"
)

// renderMarkdown renders Markdown to styled terminal text wrapped to width,
// using a fixed dark style so output is deterministic (and golden-testable). On
// any error it falls back to the raw text rather than failing the view.
func renderMarkdown(md string, width int) string {
	if strings.TrimSpace(md) == "" {
		md = "_No description yet. Press e to add one._"
	}
	if width < 10 {
		width = 10
	}
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(width))
	if err != nil {
		return md
	}
	out, err := r.Render(md)
	if err != nil {
		return md
	}
	return strings.TrimRight(out, "\n")
}
