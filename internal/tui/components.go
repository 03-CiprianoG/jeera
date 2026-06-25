package tui

import (
	_ "embed"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

// logoMosaic is a 4-row braille render of Jeera's dragon mark, generated offline
// from docs/logo.png with chafa (`--symbols braille --fg-only`). Braille packs
// 2×4 sub-cells per character, so the dragon keeps its detail in a quarter the
// rows a solid block render would need — a dotted constellation that reads as the
// dragon while staying compact in the header. It is baked in 24-bit colour but
// always rendered: Bubble Tea downsamples the colours to whatever the terminal
// supports, so the mark shows everywhere rather than vanishing on lesser
// terminals. Braille glyphs (U+2800–U+28FF) are broadly available in terminal
// fonts; where they are not, the JEERA wordmark beside it still stands.
//
//go:embed logo_mark.ans
var logoMosaic string

// components.go is Jeera's shared TUI vocabulary: the navbar pills, the
// inlaid-title bento panel, action buttons and the full-width selection row.
// Every view composes from these, so the interface reads as one material —
// learn one panel, know them all.

// --- icons -------------------------------------------------------------------

// Icons are a small, deliberately geometric set. Every glyph here is one display
// cell wide (verified against the width table), so layout math never drifts.
const (
	iconBoard   = "▦"
	iconBacklog = "≡"
	iconSprints = "◴"
	iconRuns    = "▶"
	iconHelp    = "?"
	iconSearch  = "⌕"
	iconAdd     = "+"
	iconEdit    = "✎"
	iconRun     = "▶"
	iconDiscuss = "✦"
	iconClock   = "⏱"
	iconLink    = "↗"
	iconClip    = "◌"
	iconChevL   = "◀"
	iconChevR   = "▶"
)

// --- navbar ------------------------------------------------------------------

// navItem is one destination in the top navbar: an icon and its label.
type navItem struct {
	icon  string
	label string
}

// brandMark is Jeera's logotype: the name set in spaced, bold, uppercase iris —
// a quiet wordmark that anchors the header's left edge while the navbar leads
// from the right. The letter-spacing is what reads as "logo" rather than "label"
// in a terminal that has no real display typeface.
func brandMark(t theme.Theme) string {
	return lipgloss.NewStyle().
		Foreground(t.P.FocusGlow).
		Bold(true).
		Render("J E E R A")
}

// brandLogo is the full header brand: the dragon mosaic riding just left of the
// JEERA wordmark, vertically centred against it. The mosaic is baked in 24-bit
// colour, but it is always rendered — Bubble Tea's renderer downsamples the
// colours to whatever the terminal supports (256, 16, …), so the dragon shows
// everywhere rather than vanishing on non-truecolor terminals.
func brandLogo(t theme.Theme) string {
	word := brandMark(t)
	return lipgloss.JoinHorizontal(lipgloss.Center, logoMosaic, "  "+word)
}

// navbar renders the header strip: the dragon-and-JEERA logo pinned to the left
// edge and the destination pills right-aligned across the rest of the row, each
// carrying an icon and a label, with the active one filled iris and a hairline
// grounding it to the interface below. Logo left, wayfinding right.
func navbar(t theme.Theme, width int, items []navItem, active int, logo string) string {
	pills := make([]string, 0, len(items))
	for i, it := range items {
		label := it.icon + "  " + it.label
		var style lipgloss.Style
		if i == active {
			style = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(t.P.FocusGlow).
				Background(t.P.Focus).
				Foreground(t.P.BgBase).
				Bold(true).
				Padding(0, 2)
		} else {
			style = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(t.P.Border).
				Foreground(t.P.TextMuted).
				Padding(0, 2)
		}
		pills = append(pills, style.Render(label))
	}

	// A blank gap column between pills keeps them distinct without a separator.
	blocks := make([]string, 0, len(pills)*2)
	for i, p := range pills {
		if i > 0 {
			blocks = append(blocks, "   ")
		}
		blocks = append(blocks, p)
	}
	row := lipgloss.JoinHorizontal(lipgloss.Center, blocks...)

	// The band is as tall as the taller of the logo and the pills; the shorter
	// of the two is vertically centred within it. The dragon mosaic is taller
	// than the pill row, so it sets the height and the pills float centred.
	bandH := max(lipgloss.Height(row), lipgloss.Height(logo))

	// The header carries a 2-cell frame on each side (added back by Padding
	// below), so logo + pills share the interior width.
	innerW := max(0, width-4)
	logoBlock := lipgloss.Place(lipgloss.Width(logo), bandH, lipgloss.Left, lipgloss.Center, logo)
	navW := max(1, innerW-lipgloss.Width(logoBlock))
	navBlock := lipgloss.Place(navW, bandH, lipgloss.Right, lipgloss.Center, row)

	inner := lipgloss.JoinHorizontal(lipgloss.Center, logoBlock, navBlock)
	band := lipgloss.NewStyle().Padding(0, 2).Render(inner)
	rule := lipgloss.NewStyle().Foreground(t.P.Border).Render(strings.Repeat("─", max(0, width)))
	return lipgloss.JoinVertical(lipgloss.Left, band, rule)
}

// --- panel -------------------------------------------------------------------

// panelOpts configures a bento panel: a rounded box whose section title is
// inlaid into the top edge, like a fieldset legend. The focused panel glows iris
// on both its border and its title, so the eye always knows where TAB landed.
type panelOpts struct {
	title   string
	body    string // pre-rendered interior content; lines are clipped/padded to fit
	width   int
	height  int
	focused bool
	accent  color.Color // optional title/edge tint (e.g. a status colour); overrides the default
}

// panel draws a titled, focus-aware box exactly width×height cells. It is the
// unit of the ticket bento and Jeera's signature element.
func panel(t theme.Theme, p panelOpts) string {
	if p.width < 6 {
		p.width = 6
	}
	if p.height < 3 {
		p.height = 3
	}
	textW := p.width - 4   // a 1-cell border + 1-cell pad on each side
	innerH := p.height - 2 // top + bottom border rows

	borderC := t.P.Border
	titleC := t.P.TextMuted
	if p.focused {
		borderC, titleC = t.P.FocusGlow, t.P.FocusGlow
	}
	if p.accent != nil {
		titleC = p.accent
		if p.focused {
			borderC = p.accent
		}
	}
	bs := lipgloss.NewStyle().Foreground(borderC)
	ts := lipgloss.NewStyle().Foreground(titleC)
	if p.focused {
		ts = ts.Bold(true)
	}

	lines := make([]string, 0, innerH+2)
	lines = append(lines, panelTop(p.title, p.width, bs, ts))
	for _, row := range fitLines(p.body, textW, innerH) {
		lines = append(lines, bs.Render("│")+" "+row+" "+bs.Render("│"))
	}
	lines = append(lines, bs.Render("╰"+strings.Repeat("─", p.width-2)+"╯"))
	return strings.Join(lines, "\n")
}

// panelTop builds the top border with the title inlaid after the left corner:
// ╭─ TITLE ───────────╮. It falls back to a plain edge when the box is too
// narrow to seat a legible title.
func panelTop(title string, width int, bs, ts lipgloss.Style) string {
	if title == "" || width < 10 {
		return bs.Render("╭" + strings.Repeat("─", width-2) + "╮")
	}
	mid := width - 3 // cells between "╭─" and "╮"
	up := strings.ToUpper(title)
	if lipgloss.Width(up) > mid-2 {
		up = ansi.Truncate(up, mid-2, "…")
	}
	k := mid - 2 - lipgloss.Width(up)
	if k < 0 {
		k = 0
	}
	return bs.Render("╭─ ") + ts.Render(up) + bs.Render(" "+strings.Repeat("─", k)+"╮")
}

// fitLines clips/pads a body block to exactly innerH rows, each exactly textW
// display cells. Truncation is ANSI-aware, so a styled row never breaks an
// escape; padding is plain (it sits on the app background).
func fitLines(body string, textW, innerH int) []string {
	if textW < 1 {
		textW = 1
	}
	src := strings.Split(body, "\n")
	out := make([]string, 0, innerH)
	for i := 0; i < innerH; i++ {
		line := ""
		if i < len(src) {
			line = src[i]
		}
		if lipgloss.Width(line) > textW {
			line = ansi.Truncate(line, textW, "…")
		}
		if pad := textW - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		out = append(out, line)
	}
	return out
}

// sectionTitle styles a sub-heading inside a panel — uppercase and quiet, the
// same register as a panel's inlaid legend.
func sectionTitle(t theme.Theme, s string) string {
	return t.PanelTitle.Render(strings.ToUpper(s))
}

// sectionHeader is a list view's heading: an inset, uppercase section title with
// a muted count beside it, aligned with the inset rows beneath it.
func sectionHeader(t theme.Theme, title, count string) string {
	h := "  " + sectionTitle(t, title)
	if count != "" {
		h += "   " + t.HelpDesc.Render(count)
	}
	return h
}

// ansiClip truncates a (possibly styled) string to w display cells, preserving
// ANSI escapes and adding an ellipsis when it cuts.
func ansiClip(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}

// --- buttons -----------------------------------------------------------------

// button renders an action as a pill. The focused one carries the iris fill, so
// it reads as "what Enter does here".
func button(t theme.Theme, label string, focused bool) string {
	if focused {
		return t.ButtonFocus.Render(label)
	}
	return t.Button.Render(label)
}

// buttonRow lays out buttons left-to-right with a gap, marking index `focus`
// (or none when focus is negative).
func buttonRow(t theme.Theme, labels []string, focus int) string {
	parts := make([]string, 0, len(labels)*2)
	for i, l := range labels {
		if i > 0 {
			parts = append(parts, " ")
		}
		parts = append(parts, button(t, l, i == focus))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// --- modal shell -------------------------------------------------------------

// modalShell assembles Jeera's standard dialog chrome so every overlay reads as
// one family: a titled header, a hairline that separates it from the body, the
// body itself (padded to a calm minimum so dialogs have presence), and a hint
// footer beneath a second hairline. The two hairlines are the only structural
// flourish — quiet, in the subtle border tone — and they zone the dialog into
// header / body / footer the way a well-set form does.
//
// width is the modal's full width (rounded border + Padding(1,2) included);
// minBodyH floors the body slot's height. subtitle is plain text (styled here);
// hint is rendered by the caller so it can mix accents (use modalHint for the
// common dim form). Pass an empty subtitle or hint to drop that line.
func modalShell(t theme.Theme, width, minBodyH int, title, subtitle, body, hint string) string {
	inner := width - 6 // RoundedBorder (2 cols) + Padding(1,2) (4 cols)
	if inner < 1 {
		inner = 1
	}
	rule := lipgloss.NewStyle().Foreground(t.P.Border).Render(strings.Repeat("─", inner))

	header := t.Title.Render(title)
	if subtitle != "" {
		header += "\n" + t.HelpDesc.Render(subtitle)
	}

	// Pad the body up to its floor so the dialog keeps a consistent, unhurried
	// height regardless of how little it carries.
	if pad := minBodyH - lipgloss.Height(body); pad > 0 {
		body += strings.Repeat("\n", pad)
	}

	parts := []string{header, rule, "", body, "", rule}
	if hint != "" {
		parts = append(parts, hint)
	}
	return t.Modal.Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

// modalHint styles a footer hint in the standard quiet tone. Callers that mix
// accented keys (y/n, ▸) build their own string instead.
func modalHint(t theme.Theme, s string) string { return t.HelpDesc.Render(s) }

// Standard dialog widths. Bigger than the content strictly needs — the extra
// air is the point — and centralised so the family stays in proportion.
const (
	modalWidthForm     = 72
	modalWidthList     = 64 // picker, projects, search
	modalWidthConfirm  = 58
	modalWidthSettings = 70
	modalWidthMCP      = 80
	modalWidthHelp     = 98
)

// --- value cycler ------------------------------------------------------------

// cycler renders a cyclable value. When its row is focused it wears chevrons —
// ◀ value ▶ — to advertise that left/right change it; otherwise it sits flush,
// indented to keep the value column aligned with the focused state.
func cycler(t theme.Theme, value string, valStyle lipgloss.Style, focused bool) string {
	if focused {
		ch := lipgloss.NewStyle().Foreground(t.P.FocusGlow)
		return ch.Render(iconChevL+" ") + valStyle.Render(value) + ch.Render(" "+iconChevR)
	}
	return "  " + valStyle.Render(value)
}

// --- full-width selection row ------------------------------------------------

// cell is one styled span of a list row. Building rows from spans lets the
// selection fill cover every cell — text, separators and trailing pad — so the
// highlight reads as one continuous bar rather than a ragged underline.
type cell struct {
	text string
	fg   color.Color // nil → the row's default foreground
	bold bool
}

func cText(text string) cell               { return cell{text: text} }
func cFg(text string, c color.Color) cell  { return cell{text: text, fg: c} }
func cKey(text string, c color.Color) cell { return cell{text: text, fg: c, bold: true} }

// listRow renders left and right cell groups as one full-width line. When
// selected the whole row — gaps and pad included — carries the iris selection
// fill, fixing the "row doesn't span the width" feel of a marker-only highlight.
func listRow(t theme.Theme, width int, selected bool, left, right []cell) string {
	var bg color.Color
	if selected {
		bg = t.P.BgSelect
	}
	render := func(cells []cell) (string, int) {
		var b strings.Builder
		w := 0
		for _, c := range cells {
			st := lipgloss.NewStyle()
			if c.fg != nil {
				st = st.Foreground(c.fg)
			} else {
				st = st.Foreground(t.P.TextPrimary)
			}
			if bg != nil {
				st = st.Background(bg)
			}
			if c.bold {
				st = st.Bold(true)
			}
			b.WriteString(st.Render(c.text))
			w += lipgloss.Width(c.text)
		}
		return b.String(), w
	}
	leftStr, lw := render(left)
	rightStr, rw := render(right)
	gap := width - lw - rw
	if gap < 1 {
		gap = 1
	}
	fillStyle := lipgloss.NewStyle()
	if bg != nil {
		fillStyle = fillStyle.Background(bg)
	}
	return leftStr + fillStyle.Render(strings.Repeat(" ", gap)) + rightStr
}
