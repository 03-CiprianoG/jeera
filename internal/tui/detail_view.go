package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

func (d *detailModel) View() string {
	t := d.theme
	head := d.renderHead()
	rule := lipgloss.NewStyle().Foreground(t.P.Border).Render(strings.Repeat("─", d.width))
	body := lipgloss.JoinHorizontal(lipgloss.Top, d.renderLeft(), " ", d.renderSidebar())
	comments := d.renderComments()
	footer := d.renderFooter()
	content := lipgloss.JoinVertical(lipgloss.Left, head, rule, body, comments, footer)
	return fitHeight(content, d.height)
}

func (d *detailModel) renderHead() string {
	t := d.theme
	name, cat := d.statusInfo()
	dot := lipgloss.NewStyle().Foreground(t.CategoryColor(cat)).Render("●")
	left := t.CardKey.Render(d.issue.Key) + "  " +
		t.CardMeta.Render(string(d.issue.Type)) + "  " +
		dot + " " + lipgloss.NewStyle().Foreground(t.CategoryColor(cat)).Render(name)
	right := t.HelpDesc.Render("esc back")
	inner := spread(left, right, d.width-2)
	return lipgloss.NewStyle().Background(t.P.BgSurface).Width(d.width).Render(" " + inner + " ")
}

func (d *detailModel) renderLeft() string {
	t := d.theme
	title := t.Title.Width(d.descWidth()).Render(truncate(d.issue.Title, d.descWidth()*2))
	var pane string
	if d.mode == dEditDesc {
		pane = d.desc.View()
	} else {
		pane = d.vp.View()
	}
	left := lipgloss.JoinVertical(lipgloss.Left, title, "", pane)
	return lipgloss.NewStyle().Width(d.descWidth()).Render(left)
}

func (d *detailModel) renderSidebar() string {
	t := d.theme
	w := d.width - d.descWidth() - 3
	if w < 16 {
		w = 16
	}
	lines := make([]string, 0, int(dfFieldCount)+4)
	for f := detailField(0); f < dfFieldCount; f++ {
		label, value, c := d.fieldDisplay(f)
		labelStyle := t.Label
		valStyle := lipgloss.NewStyle().Foreground(c)
		marker := "  "
		if f == d.field && d.mode == dViewing {
			marker = t.HelpKey.Render("▸ ")
			labelStyle = labelStyle.Bold(true)
		}
		row := marker + labelStyle.Render(label) + " " + valStyle.Render(truncate(value, w-len(label)-4))
		lines = append(lines, row)
	}
	// Links section.
	if len(d.links) > 0 {
		lines = append(lines, "", t.Label.Render("Links"))
		for _, l := range d.links {
			lines = append(lines, "  "+t.CardMeta.Render(string(l.Type))+" "+t.CardKey.Render(l.Issue.Key))
		}
	}

	// Worktree + runs.
	wt := "on"
	if d.issue.WorktreeOn != nil && !*d.issue.WorktreeOn {
		wt = "off"
	}
	lines = append(lines, "", t.Label.Render("Worktree")+" "+t.StatusText.Render(wt))
	if len(d.runs) > 0 {
		lines = append(lines, t.Label.Render("Runs"))
		for i, r := range d.runs {
			if i >= 3 {
				break
			}
			rs := lipgloss.NewStyle().Foreground(t.RunStateColor(r.Status))
			lines = append(lines, "  "+t.CardMeta.Render(fmt.Sprintf("v%d", r.Version))+" "+rs.Render(string(r.Status)))
		}
	}

	body := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return lipgloss.NewStyle().Width(w).Render(fitHeight(body, d.bodyHeight()))
}

func (d *detailModel) renderComments() string {
	t := d.theme
	lines := []string{t.Label.Render("Activity")}
	start := 0
	if len(d.comments) > 4 {
		start = len(d.comments) - 4
	}
	for _, c := range d.comments[start:] {
		who := t.Chip.Render(c.Author)
		lines = append(lines, "  "+who+" "+t.CardMeta.Render(truncate(c.Body, d.width-len(c.Author)-6)))
	}
	return fitHeight(lipgloss.JoinVertical(lipgloss.Left, lines...), d.commentsHeight())
}

func (d *detailModel) renderFooter() string {
	t := d.theme
	var hint string
	switch d.mode {
	case dEditDesc:
		hint = t.HelpKey.Render("ctrl+s") + " " + t.HelpDesc.Render("save") + "   " +
			t.HelpKey.Render("esc") + " " + t.HelpDesc.Render("cancel")
	case dInput:
		hint = t.Label.Render(d.inputLabel()) + " " + d.input.View() + "   " +
			t.HelpKey.Render("enter") + " " + t.HelpDesc.Render("save")
	default:
		segs := []struct{ k, v string }{
			{"j/k", "field"}, {"h/l", "change"}, {"e", "describe"}, {"c", "comment"},
			{"s", "start"}, {"w", "worktree"}, {"esc", "back"},
		}
		parts := make([]string, 0, len(segs))
		for _, s := range segs {
			parts = append(parts, t.HelpKey.Render(s.k)+" "+t.HelpDesc.Render(s.v))
		}
		hint = strings.Join(parts, t.HelpDesc.Render("  "))
	}
	right := ""
	if d.err != "" {
		right = t.Error.Render("! " + truncate(d.err, d.width/3))
	}
	inner := spread(truncate(hint, d.width-2-lipgloss.Width(right)-1), right, d.width-2)
	return lipgloss.NewStyle().Background(t.P.BgSurface).Width(d.width).Render(" " + inner + " ")
}

func (d *detailModel) inputLabel() string {
	switch d.inputKind {
	case ikPoints:
		return "Points:"
	case ikTag:
		return "Add tag:"
	default:
		return "Comment:"
	}
}

// statusInfo returns the issue's status name and category.
func (d *detailModel) statusInfo() (string, core.StatusCategory) {
	for _, s := range d.statuses {
		if s.ID == d.issue.StatusID {
			return s.Name, s.Category
		}
	}
	return "—", core.CategoryTodo
}

func (d *detailModel) fieldDisplay(f detailField) (label, value string, c color.Color) {
	t := d.theme
	none := "—"
	switch f {
	case dfStatus:
		name, cat := d.statusInfo()
		return "Status", name, t.CategoryColor(cat)
	case dfType:
		return "Type", string(d.issue.Type), t.P.TextPrimary
	case dfPriority:
		return "Priority", theme.PriorityGlyph(d.issue.Priority) + " " + string(d.issue.Priority), t.PriorityColor(d.issue.Priority)
	case dfPoints:
		v := none
		if d.issue.StoryPoints != nil {
			v = fmt.Sprintf("%d", *d.issue.StoryPoints)
		}
		return "Points", v, t.P.TextPrimary
	case dfProvider:
		return "Provider", orNone(string(d.issue.Assignee.Provider), none), t.P.Info
	case dfModel:
		return "Model", orNone(d.issue.Assignee.Model, none), t.P.Info
	case dfEffort:
		return "Effort", orNone(string(d.issue.Assignee.Effort), none), t.P.Info
	case dfSprint:
		return "Sprint", d.sprintName(none), t.P.TextPrimary
	case dfEpic:
		return "Epic", d.epicKey(none), t.P.TextPrimary
	case dfTags:
		if len(d.issueTags) == 0 {
			return "Tags", none, t.P.TextSubtle
		}
		names := make([]string, len(d.issueTags))
		for i, tg := range d.issueTags {
			names[i] = "#" + tg.Name
		}
		return "Tags", strings.Join(names, " "), t.P.TextSubtle
	}
	return "", "", t.P.TextPrimary
}

func (d *detailModel) sprintName(none string) string {
	if d.issue.SprintID == nil {
		return none
	}
	for _, s := range d.sprints {
		if s.ID == *d.issue.SprintID {
			return s.Name
		}
	}
	return none
}

func (d *detailModel) epicKey(none string) string {
	if d.issue.EpicID == nil {
		return none
	}
	for _, e := range d.epics {
		if e.ID == *d.issue.EpicID {
			return e.Key
		}
	}
	return none
}

func orNone(s, none string) string {
	if s == "" {
		return none
	}
	return s
}
