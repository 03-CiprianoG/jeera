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
	head := d.renderHead()
	tabs := tabStrip(d.theme, d.width, detailTabLabels, int(d.tab))
	body := fitHeight(d.renderBody(), d.bodyHeight())
	footer := d.renderFooter()
	content := lipgloss.JoinVertical(lipgloss.Left, head, tabs, body, footer)
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

// renderBody dispatches to the active tab's content.
func (d *detailModel) renderBody() string {
	switch d.tab {
	case tabAgent:
		return d.renderAgent()
	case tabRelations:
		return d.renderRelations()
	case tabFiles:
		return d.renderFiles()
	case tabActivity:
		return d.renderActivity()
	default:
		return d.renderOverview()
	}
}

// --- Overview ----------------------------------------------------------------

func (d *detailModel) renderOverview() string {
	w := d.sidebarWidth()
	fields := lipgloss.JoinVertical(lipgloss.Left, d.fieldLines(tabOverview.fields(), w)...)
	right := lipgloss.NewStyle().Width(w).Render(fields)
	return lipgloss.JoinHorizontal(lipgloss.Top, d.renderDescPane(), " ", right)
}

func (d *detailModel) renderDescPane() string {
	t := d.theme
	title := t.Title.Width(d.descWidth()).Render(truncate(d.issue.Title, d.descWidth()*2))
	pane := d.vp.View()
	if d.mode == dEditDesc {
		pane = d.desc.View()
	}
	left := lipgloss.JoinVertical(lipgloss.Left, title, "", pane)
	return lipgloss.NewStyle().Width(d.descWidth()).Render(left)
}

func (d *detailModel) sidebarWidth() int {
	w := d.width - d.descWidth() - 3
	if w < 16 {
		w = 16
	}
	return w
}

// fieldLines renders the given metadata fields as one row each, marking the one
// under the cursor. Shared by the Overview and Agent tabs.
func (d *detailModel) fieldLines(fields []detailField, w int) []string {
	t := d.theme
	lines := make([]string, 0, len(fields))
	for _, f := range fields {
		label, value, c := d.fieldDisplay(f)
		labelStyle := t.Label
		valStyle := lipgloss.NewStyle().Foreground(c)
		marker := "  "
		if f == d.field && d.mode == dViewing {
			marker = t.HelpKey.Render("▸ ")
			labelStyle = labelStyle.Bold(true)
		}
		row := marker + labelStyle.Render(fmt.Sprintf("%-8s", label)) + " " + valStyle.Render(truncate(value, w-12))
		lines = append(lines, row)
	}
	return lines
}

// --- Agent -------------------------------------------------------------------

func (d *detailModel) renderAgent() string {
	t := d.theme
	colW := d.width/2 - 2
	if colW < 20 {
		colW = 20
	}

	left := []string{t.Title.Render("Assignee"), ""}
	left = append(left, d.fieldLines(tabAgent.fields(), colW)...)
	wt := "on"
	if d.issue.WorktreeOn != nil && !*d.issue.WorktreeOn {
		wt = "off"
	}
	// Align the value to the same column as the assignee fields above (2-cell
	// marker gutter + an 8-wide label).
	left = append(left, "", "  "+t.Label.Render(fmt.Sprintf("%-8s", "Worktree"))+" "+t.StatusText.Render(wt))
	leftCol := lipgloss.NewStyle().Width(colW).Render(lipgloss.JoinVertical(lipgloss.Left, left...))

	right := []string{t.Title.Render("Runs"), ""}
	if len(d.runs) == 0 {
		right = append(right, t.HelpDesc.Render("— none yet —"))
	}
	for _, r := range d.runs {
		rs := lipgloss.NewStyle().Foreground(t.RunStateColor(r.Status))
		when := ""
		if r.StartedAt != nil {
			when = "  " + t.CardMeta.Render(r.StartedAt.Local().Format("Jan 2 15:04"))
		}
		right = append(right, t.CardMeta.Render(fmt.Sprintf("v%-3d", r.Version))+" "+rs.Render(string(r.Status))+when)
	}
	if len(d.schedules) > 0 {
		right = append(right, "", t.Title.Render("Schedules"), "")
		for _, sc := range d.schedules {
			when := ""
			if sc.NextRun != nil {
				when = " → " + sc.NextRun.Local().Format("Jan 2 15:04")
			}
			spec := sc.CronSpec
			if !sc.Enabled {
				spec += " (off)"
			}
			right = append(right, t.Chip.Render("⏱ "+spec)+t.HelpDesc.Render(when))
		}
	}
	rightCol := lipgloss.NewStyle().Width(d.width - colW - 2).Render(lipgloss.JoinVertical(lipgloss.Left, right...))

	return lipgloss.JoinHorizontal(lipgloss.Top, leftCol, " ", rightCol)
}

// --- Relations ---------------------------------------------------------------

func (d *detailModel) renderRelations() string {
	t := d.theme
	// One grid for the whole tab: every row carries a 2-cell indent then a column
	// padded to a common width, so the eye tracks a single left edge throughout.
	row := func(col, value string) string {
		return "  " + t.Label.Render(fmt.Sprintf("%-9s", col)) + " " + t.StatusText.Render(value)
	}

	parent := "—"
	if d.parent != nil {
		parent = d.parent.Key + "  " + d.parent.Title
	}
	lines := []string{
		t.Title.Render("Hierarchy"), "",
		row("Epic", d.epicKey("—")),
		row("Parent", truncate(parent, d.width-13)),
		"", t.Title.Render("Children"), "",
	}
	if len(d.children) == 0 {
		lines = append(lines, "  "+t.HelpDesc.Render("— none —"))
	}
	for _, c := range d.children {
		lines = append(lines, "  "+t.CardKey.Render(fmt.Sprintf("%-9s", c.Key))+" "+t.StatusText.Render(truncate(c.Title, d.width-13)))
	}

	lines = append(lines, "", t.Title.Render("Links"), "")
	if len(d.links) == 0 {
		lines = append(lines, "  "+t.HelpDesc.Render("— none —"))
	}
	for _, l := range d.links {
		lines = append(lines, "  "+t.CardMeta.Render(fmt.Sprintf("%-11s", string(l.Type)))+" "+
			t.CardKey.Render(l.Issue.Key)+" "+t.StatusText.Render(truncate(l.Issue.Title, d.width-26)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// --- Files -------------------------------------------------------------------

func (d *detailModel) renderFiles() string {
	t := d.theme
	if len(d.attachments) == 0 {
		return t.HelpDesc.Render("No attachments yet. Press A to add a link or file.")
	}
	lines := []string{t.Label.Render(fmt.Sprintf("%d attachment%s", len(d.attachments), pluralS(len(d.attachments)))), ""}
	for i, a := range d.attachments {
		marker := "  "
		nameStyle := t.StatusText
		if i == d.attachSel {
			marker = t.HelpKey.Render("▸ ")
			nameStyle = t.CardTitle
		}
		icon := "📎"
		meta := a.MIME
		if a.IsURL() {
			icon = "🔗"
			meta = a.Path
		}
		// The meta column is subordinate to the name and shrinks with the width, so
		// a long URL never collides with the filename on a narrow terminal.
		metaW := d.width / 3
		if metaW > 38 {
			metaW = 38
		}
		nameW := d.width - metaW - 8
		if nameW < 8 {
			nameW = 8
		}
		row := marker + icon + " " + nameStyle.Render(truncate(a.Filename, nameW))
		if meta != "" {
			row = spread(row, t.CardMeta.Render(truncate(meta, metaW)), d.width-2)
		}
		lines = append(lines, row)
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// --- Activity ----------------------------------------------------------------

func (d *detailModel) renderActivity() string {
	t := d.theme
	if len(d.comments) == 0 {
		return t.HelpDesc.Render("No activity yet. Press c to add a comment.")
	}
	// Show the most recent comments that fit the body region.
	start := 0
	if max := d.bodyHeight(); len(d.comments) > max {
		start = len(d.comments) - max
	}
	const authorW = 8 // a fixed author gutter so every comment body shares one left edge
	lines := make([]string, 0, len(d.comments)-start)
	for _, c := range d.comments[start:] {
		name := truncate(c.Author, authorW)
		who := t.Chip.Render(name + strings.Repeat(" ", authorW-lipgloss.Width(name)))
		lines = append(lines, who+"  "+t.StatusText.Render(truncate(c.Body, d.width-authorW-4)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// --- Footer ------------------------------------------------------------------

func (d *detailModel) renderFooter() string {
	t := d.theme
	seg := func(k, v string) string { return t.HelpKey.Render(k) + " " + t.HelpDesc.Render(v) }

	right := ""
	switch {
	case d.err != "":
		right = t.Error.Render("! " + truncate(d.err, d.width/3))
	case d.notice != "":
		right = t.Toast.Render(truncate(d.notice, d.width/3))
	}
	budget := d.width - 3 - lipgloss.Width(right)
	if budget < 0 {
		budget = 0
	}

	var hint string
	switch d.mode {
	case dInput:
		// The live input manages its own width; show it with the save/cancel keys.
		hint = t.Label.Render(d.inputLabel()) + " " + d.input.View() + "   " + seg("enter", "save") + "  " + seg("esc", "cancel")
	case dEditDesc:
		hint = fitSegments([]string{seg("ctrl+s", "save"), seg("esc", "cancel")}, budget, t)
	default:
		hint = fitSegments(d.tabHints(seg), budget, t)
	}

	inner := spread(hint, right, d.width-2)
	return lipgloss.NewStyle().Background(t.P.BgSurface).Width(d.width).Render(" " + inner + " ")
}

// tabHints lists the footer keybar segments for the active tab — only the keys
// that tab actually responds to, so the footer never advertises an inert action.
func (d *detailModel) tabHints(seg func(k, v string) string) []string {
	parts := []string{seg("tab", "tabs")}
	switch d.tab {
	case tabOverview:
		parts = append(parts, seg("j/k", "field"), seg("h/l", "change"), seg("e", "describe"))
		if d.field == dfPoints || d.field == dfTags {
			parts = append(parts, seg("enter", "edit"))
		}
	case tabAgent:
		parts = append(parts, seg("j/k", "field"), seg("h/l", "change"),
			seg("s", "start"), seg("D", "+children"), seg("d", "discuss"), seg("S", "schedule"), seg("w", "worktree"))
	case tabFiles:
		parts = append(parts, seg("j/k", "select"), seg("o", "open"), seg("A", "attach"))
	case tabActivity:
		parts = append(parts, seg("c", "comment"))
	}
	return append(parts, seg("esc", "back"))
}

// fitSegments joins as many whole hint segments as fit within budget display
// cells. Dropping whole (already-styled) segments rather than cutting the string
// keeps the footer one clean row at any width and never slices an ANSI escape.
func fitSegments(segs []string, budget int, t theme.Theme) string {
	sep := t.HelpDesc.Render("  ")
	var b strings.Builder
	w := 0
	for _, s := range segs {
		add := lipgloss.Width(s)
		if b.Len() > 0 {
			add += lipgloss.Width(sep)
		}
		if b.Len() > 0 && w+add > budget {
			break
		}
		if b.Len() > 0 {
			b.WriteString(sep)
		}
		b.WriteString(s)
		w += add
	}
	return b.String()
}

func (d *detailModel) inputLabel() string {
	switch d.inputKind {
	case ikPoints:
		return "Points:"
	case ikTag:
		return "Add tag:"
	case ikCron:
		return "Schedule:"
	case ikAttach:
		return "Attach:"
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
