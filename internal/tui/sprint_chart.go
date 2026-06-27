package tui

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

// sprint_chart.go draws the Sprint detail's two data graphics: a horizontal
// progress meter and the burndown line chart. Both are hand-drawn from glyphs
// and theme tokens (no charting dependency) so the output is deterministic and
// golden-testable, and so the burndown can plot in braille — the same 2×4
// sub-cell trick the dragon mosaic uses — giving a real line at a quarter of the
// vertical cost of block columns.

// meter renders a horizontal progress bar `width` cells wide: a filled run in
// fillC over a track in the subtle border tone. value/total sets the fill; any
// non-zero progress shows at least a sliver, so "1 of 30" never reads as empty.
func meter(t theme.Theme, value, total, width int, fillC color.Color) string {
	if width < 1 {
		return ""
	}
	frac := 0.0
	if total > 0 {
		frac = float64(value) / float64(total)
	}
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	filled := int(math.Round(frac * float64(width)))
	if value > 0 && filled == 0 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	fill := lipgloss.NewStyle().Foreground(fillC).Render(strings.Repeat("█", filled))
	track := lipgloss.NewStyle().Foreground(t.P.Border).Render(strings.Repeat("░", width-filled))
	return fill + track
}

// --- braille canvas ----------------------------------------------------------

// brailleDot maps a sub-cell (dy row 0-3, dx col 0-1) to its braille bit. A
// braille glyph packs a 2×4 dot matrix, so a w×h cell canvas plots a 2w×4h dot
// field — the resolution that lets a short panel still carry a legible curve.
var brailleDot = [4][2]uint8{
	{0x01, 0x08},
	{0x02, 0x10},
	{0x04, 0x20},
	{0x40, 0x80},
}

// brailleCanvas is a w×h grid of braille cells addressed in dot space, with the
// origin at the top-left. It is a tiny raster target for plotting one series.
type brailleCanvas struct {
	w, h int
	bits [][]uint8
}

func newBrailleCanvas(w, h int) *brailleCanvas {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	bits := make([][]uint8, h)
	for i := range bits {
		bits[i] = make([]uint8, w)
	}
	return &brailleCanvas{w: w, h: h, bits: bits}
}

// dotW and dotH are the canvas dimensions in dots.
func (c *brailleCanvas) dotW() int { return c.w * 2 }
func (c *brailleCanvas) dotH() int { return c.h * 4 }

// set lights the dot at (px, py) in dot space, ignoring out-of-bounds points so
// callers can plot without clamping every coordinate themselves.
func (c *brailleCanvas) set(px, py int) {
	if px < 0 || py < 0 {
		return
	}
	col, row := px/2, py/4
	if col >= c.w || row >= c.h {
		return
	}
	c.bits[row][col] |= brailleDot[py%4][px%2]
}

// line plots a straight segment between two dots (Bresenham), so successive
// data points read as a continuous line rather than scattered marks.
func (c *brailleCanvas) line(x0, y0, x1, y1 int) {
	dx := iabs(x1 - x0)
	dy := -iabs(y1 - y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		c.set(x0, y0)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// mergeCanvases renders two plotted canvases into h rows of glyphs. Where only
// one series lights a cell it takes that series' colour; where both do, the
// dots are OR'd into one glyph and the lead series (a) wins the colour — so the
// actual line always reads over the ideal guideline it crosses.
func mergeCanvases(a, b *brailleCanvas, aC, bC color.Color) []string {
	aSt := lipgloss.NewStyle().Foreground(aC)
	bSt := lipgloss.NewStyle().Foreground(bC)
	rows := make([]string, a.h)
	for r := 0; r < a.h; r++ {
		var sb strings.Builder
		for col := 0; col < a.w; col++ {
			ab := a.bits[r][col]
			bb := b.bits[r][col]
			if ab == 0 && bb == 0 {
				sb.WriteByte(' ')
				continue
			}
			glyph := string(rune(0x2800 + int(ab|bb)))
			if ab != 0 {
				sb.WriteString(aSt.Render(glyph))
			} else {
				sb.WriteString(bSt.Render(glyph))
			}
		}
		rows[r] = sb.String()
	}
	return rows
}

// --- burndown ----------------------------------------------------------------

// renderBurndown draws the sprint burndown into a w×h cell block: the ideal
// guideline (a straight descent from the full scope to zero across the sprint
// window) and the actual remaining work, reconstructed day by day and stopping
// at today. A y-axis with the scope and zero ticks, an x-axis spanning the
// window's dates, and an optional legend frame it. When the sprint can't be
// charted yet (not started, no dates, nothing to burn) it returns a centered
// explanation instead, so the panel never shows a misleading empty axis.
func renderBurndown(t theme.Theme, sp core.Sprint, mt sprintMetrics, loc *time.Location, w, h int) string {
	switch {
	case !mt.started:
		return burndownNote(t, w, h, "The burndown begins when the sprint starts.", "Start it from Progress →.")
	case !mt.hasWindow:
		return burndownNote(t, w, h, "Set start and end dates to chart the burndown.", "Edit them from Progress →.")
	case mt.basisTotal == 0:
		return burndownNote(t, w, h, "Nothing to burn down yet.", "Add issues or estimate them in points.")
	}

	// The y-axis gutter is as wide as the larger tick label plus its tick column.
	topLabel := fmt.Sprintf("%d", mt.basisTotal)
	const botLabel = "0"
	gutterW := imax(len(topLabel), len(botLabel)) + 1
	plotW := w - gutterW
	// Reserve two rows for the x-axis (its rule and the date labels); spend one
	// more on a legend only when the panel is tall and wide enough to earn it.
	legend := h >= 7 && w >= 26
	plotH := h - 2
	if legend {
		plotH--
	}
	if plotW < 6 || plotH < 2 {
		return burndownNote(t, w, h, "Make the window taller", "to see the burndown.")
	}

	actual := newBrailleCanvas(plotW, plotH)
	ideal := newBrailleCanvas(plotW, plotH)
	dotW, dotH := actual.dotW(), actual.dotH()
	px := func(day int) int {
		return iclamp(int(math.Round(float64(day)/float64(mt.dayTotal)*float64(dotW-1))), 0, dotW-1)
	}
	py := func(val float64) int {
		return iclamp(int(math.Round((1-val/float64(mt.basisTotal))*float64(dotH-1))), 0, dotH-1)
	}

	ideal.line(px(0), py(float64(mt.basisTotal)), px(mt.dayTotal), py(0))

	prevX, prevY := -1, -1
	for _, p := range mt.series {
		if !p.hasActual {
			break
		}
		x, y := px(p.day), py(p.actual)
		if prevX < 0 {
			actual.set(x, y)
		} else {
			actual.line(prevX, prevY, x, y)
		}
		prevX, prevY = x, y
	}

	// The actual line wears the iris accent, not Success: green means "done"
	// everywhere else, and this line is remaining work, the opposite of done.
	plot := mergeCanvases(actual, ideal, t.P.Focus, t.P.TextSubtle)

	axisStyle := lipgloss.NewStyle().Foreground(t.P.Border)
	labelStyle := t.HelpDesc

	rows := make([]string, 0, h)
	if legend {
		swatch := func(c color.Color) string { return lipgloss.NewStyle().Foreground(c).Render("━━") }
		key := swatch(t.P.Focus) + labelStyle.Render(" actual") +
			labelStyle.Render("   ") + swatch(t.P.TextSubtle) + labelStyle.Render(" ideal")
		rows = append(rows, strings.Repeat(" ", gutterW)+key)
	}
	for i, line := range plot {
		lbl := strings.Repeat(" ", gutterW-1)
		tick := axisStyle.Render("│")
		switch i {
		case 0:
			lbl, tick = fmt.Sprintf("%*s", gutterW-1, topLabel), axisStyle.Render("┤")
		case len(plot) - 1:
			lbl, tick = fmt.Sprintf("%*s", gutterW-1, botLabel), axisStyle.Render("┤")
		}
		rows = append(rows, labelStyle.Render(lbl)+tick+line)
	}
	rows = append(rows,
		strings.Repeat(" ", gutterW-1)+axisStyle.Render("└"+strings.Repeat("─", plotW)),
		strings.Repeat(" ", gutterW)+spread(
			labelStyle.Render(sp.StartAt.In(loc).Format("Jan 2")),
			labelStyle.Render(sp.EndAt.In(loc).Format("Jan 2")),
			plotW,
		),
	)
	return strings.Join(rows, "\n")
}

// burndownNote centers a two-line explanation in the plot area, for the states
// that can't be charted (future sprint, missing dates, empty scope).
func burndownNote(t theme.Theme, w, h int, title, sub string) string {
	block := lipgloss.JoinVertical(lipgloss.Center,
		t.HelpDesc.Render(title),
		"",
		lipgloss.NewStyle().Foreground(t.P.TextSubtle).Render(sub),
	)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, block)
}

// --- small int helpers (kept local to avoid colliding with the builtins) -----

func iabs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func imax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func iclamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
