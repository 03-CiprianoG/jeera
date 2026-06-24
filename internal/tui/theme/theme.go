// Package theme is Jeera's design system: a single, calm token palette and the
// styles every TUI component derives from. Nothing in the TUI should hard-code a
// color — it comes from here, so the interface stays consistent and congruent.
//
// Direction is "Slate & Iris": a deep blue-slate base (deliberately not pure
// black), soft parchment text, and one restrained iris accent reserved for focus
// and the MCP "wire". Semantic color is spent only where it carries meaning —
// status category, priority and run state.
package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// Palette holds the raw tokens. Values are dark-first hex; the TUI is dark-first
// like its peers (lazygit, k9s), and a light variant can map these later.
type Palette struct {
	BgBase    color.Color // app background
	BgSurface color.Color // raised panels (cards, columns)
	BgOverlay color.Color // modals / overlays
	Border    color.Color // subtle separators and unfocused frames
	Focus     color.Color // the single accent: focus, selection, the MCP wire

	TextPrimary color.Color
	TextMuted   color.Color
	TextSubtle  color.Color

	Success color.Color // done, healthy
	Warning color.Color // in-progress, starting
	Danger  color.Color // failure, highest priority
	Info    color.Color
}

// Theme bundles the palette with derived, ready-to-use styles.
type Theme struct {
	P Palette

	App        lipgloss.Style
	Title      lipgloss.Style
	Tab        lipgloss.Style
	TabActive  lipgloss.Style
	StatusBar  lipgloss.Style
	StatusKey  lipgloss.Style
	StatusText lipgloss.Style
	HelpKey    lipgloss.Style
	HelpDesc   lipgloss.Style

	Card         lipgloss.Style
	CardSelected lipgloss.Style
	CardKey      lipgloss.Style
	CardTitle    lipgloss.Style
	CardMeta     lipgloss.Style
	Chip         lipgloss.Style
	Tag          lipgloss.Style

	ColumnTitle lipgloss.Style
	CountBadge  lipgloss.Style

	Modal      lipgloss.Style
	Label      lipgloss.Style
	Field      lipgloss.Style
	FieldFocus lipgloss.Style

	Empty lipgloss.Style
	Error lipgloss.Style
	Toast lipgloss.Style
}

// New builds the default (dark) theme.
func New() Theme {
	p := Palette{
		BgBase:    lipgloss.Color("#14161F"),
		BgSurface: lipgloss.Color("#1C1F2B"),
		BgOverlay: lipgloss.Color("#232838"),
		Border:    lipgloss.Color("#2E3346"),
		Focus:     lipgloss.Color("#8891D9"),

		TextPrimary: lipgloss.Color("#E6E8F0"),
		TextMuted:   lipgloss.Color("#9AA0B4"),
		TextSubtle:  lipgloss.Color("#5E647A"),

		Success: lipgloss.Color("#7FB894"),
		Warning: lipgloss.Color("#D9A85C"),
		Danger:  lipgloss.Color("#D98A8A"),
		Info:    lipgloss.Color("#6FB3C9"),
	}
	return build(p)
}

func build(p Palette) Theme {
	base := lipgloss.NewStyle()
	t := Theme{P: p}

	t.App = base.Foreground(p.TextPrimary).Background(p.BgBase)
	t.Title = base.Foreground(p.TextPrimary).Bold(true)
	t.Tab = base.Foreground(p.TextSubtle).Padding(0, 2)
	t.TabActive = base.Foreground(p.Focus).Bold(true).Padding(0, 2)

	t.StatusBar = base.Foreground(p.TextMuted).Background(p.BgSurface).Padding(0, 1)
	t.StatusKey = base.Foreground(p.Focus).Bold(true)
	t.StatusText = base.Foreground(p.TextMuted)
	t.HelpKey = base.Foreground(p.Focus)
	t.HelpDesc = base.Foreground(p.TextSubtle)

	t.Card = base.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Border).
		Padding(0, 1)
	t.CardSelected = base.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Focus).
		Padding(0, 1)
	t.CardKey = base.Foreground(p.Focus).Bold(true)
	t.CardTitle = base.Foreground(p.TextPrimary)
	t.CardMeta = base.Foreground(p.TextSubtle)
	t.Chip = base.Foreground(p.Info)
	t.Tag = base.Foreground(p.TextSubtle)

	t.ColumnTitle = base.Bold(true).Padding(0, 1)
	t.CountBadge = base.Foreground(p.TextSubtle)

	t.Modal = base.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Focus).
		Background(p.BgOverlay).
		Foreground(p.TextPrimary).
		Padding(1, 2)
	t.Label = base.Foreground(p.TextMuted)
	t.Field = base.Foreground(p.TextPrimary).Background(p.BgSurface).Padding(0, 1)
	t.FieldFocus = base.Foreground(p.TextPrimary).Background(p.BgSurface).Padding(0, 1).
		Border(lipgloss.NormalBorder(), false, false, false, true).BorderForeground(p.Focus)

	t.Empty = base.Foreground(p.TextSubtle).Align(lipgloss.Center)
	t.Error = base.Foreground(p.Danger)
	t.Toast = base.Foreground(p.BgBase).Background(p.Focus).Padding(0, 1)

	return t
}

// CategoryColor returns the accent for a status category.
func (t Theme) CategoryColor(c core.StatusCategory) color.Color {
	switch c {
	case core.CategoryInProgress:
		return t.P.Warning
	case core.CategoryDone:
		return t.P.Success
	default: // todo
		return t.P.TextMuted
	}
}

// PriorityColor returns the color for a priority, a coral→dim urgency ramp.
func (t Theme) PriorityColor(p core.Priority) color.Color {
	switch p {
	case core.PriorityHighest:
		return t.P.Danger
	case core.PriorityHigh:
		return t.P.Warning
	case core.PriorityMedium:
		return t.P.TextMuted
	case core.PriorityLow:
		return t.P.Info
	default: // lowest
		return t.P.TextSubtle
	}
}

// PriorityGlyph is a compact urgency indicator: a restrained geometric ramp
// (filled-up = highest … filled-down = lowest). Pair it with PriorityColor.
func PriorityGlyph(p core.Priority) string {
	switch p {
	case core.PriorityHighest:
		return "▲"
	case core.PriorityHigh:
		return "△"
	case core.PriorityMedium:
		return "■"
	case core.PriorityLow:
		return "▽"
	default:
		return "▼"
	}
}

// RunStateColor returns the color for a run status badge.
func (t Theme) RunStateColor(s core.RunStatus) color.Color {
	switch s {
	case core.RunRunning, core.RunQueued:
		return t.P.Focus
	case core.RunSucceeded:
		return t.P.Success
	case core.RunFailed:
		return t.P.Danger
	case core.RunBlocked:
		return t.P.Warning
	default:
		return t.P.TextSubtle
	}
}
