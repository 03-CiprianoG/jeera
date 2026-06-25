package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

// dlayout holds the computed cell rectangles of the ticket bento, derived from
// the terminal size. Every panel is sized from here so the grid tiles the body
// exactly, with no gaps or overruns.
type dlayout struct {
	bodyH            int
	leftW, rightW    int
	single           bool // narrow terminals stack every panel in one column
	descH, actH      int
	propH, agH, relH int
}

func (d *detailModel) layout() dlayout {
	w, h := d.width, d.height
	bodyH := h - 2 // title bar + footer
	if bodyH < 8 {
		bodyH = 8
	}

	// Below ~80 columns the two-column bento can't breathe, so stack the panels in
	// one full-width column; the view clips to height, keeping the top panels.
	if w < 80 {
		descH := bodyH * 34 / 100
		if descH < 6 {
			descH = 6
		}
		propH, agH, relH := 9, 9, 6
		actH := bodyH - descH - propH - agH - relH
		if actH < 3 {
			actH = 3
		}
		return dlayout{bodyH: bodyH, leftW: w, single: true, descH: descH, actH: actH, propH: propH, agH: agH, relH: relH}
	}

	leftW := w * 62 / 100
	rightW := w - leftW - 1 // a 1-cell gutter between the columns
	if rightW < 24 {
		rightW = 24
		leftW = w - rightW - 1
	}
	if leftW < 24 {
		leftW = 24
	}

	// Left column: a tall Description over the Activity timeline.
	descH := bodyH * 60 / 100
	actH := bodyH - descH
	if actH < 5 {
		actH = 5
		descH = bodyH - actH
	}
	if descH < 6 {
		descH = 6
	}

	// Right column: Properties is fixed at the height of its seven rows; Agent and
	// Relations split the rest, with Agent guaranteed room for both button rows.
	propH := 9
	if propH > bodyH-12 {
		propH = bodyH - 12
	}
	if propH < 5 {
		propH = 5
	}
	remaining := bodyH - propH
	agH := remaining * 48 / 100
	if agH < 9 {
		agH = 9
	}
	relH := remaining - agH
	if relH < 6 {
		relH = 6
		agH = remaining - relH
		if agH < 7 {
			agH = 7
			relH = remaining - agH
		}
	}
	return dlayout{bodyH: bodyH, leftW: leftW, rightW: rightW, descH: descH, actH: actH, propH: propH, agH: agH, relH: relH}
}

// rightInner is the interior text width of a right-column panel — or the single
// column's width when the layout has collapsed to one column.
func (L dlayout) rightInner() int {
	if L.single {
		return L.leftW - 4
	}
	return L.rightW - 4
}

func (d *detailModel) descInteriorWidth() int {
	w := d.layout().leftW - 4
	if w < 10 {
		w = 10
	}
	return w
}

// descViewportHeight reserves the description panel's interior for a two-line
// title, a blank, and the trailing Edit button — the rest scrolls the body.
func (d *detailModel) descViewportHeight() int {
	h := d.layout().descH - 6
	if h < 1 {
		h = 1
	}
	return h
}

func (d *detailModel) View() string {
	title := d.renderTitleBar()
	body := d.renderBento()
	footer := d.renderFooter()
	content := lipgloss.JoinVertical(lipgloss.Left, title, body, footer)
	return fitHeight(content, d.height)
}

// renderTitleBar is the always-on identity row: key, type and live status on the
// left; how to leave on the right.
func (d *detailModel) renderTitleBar() string {
	t := d.theme
	name, cat := d.statusInfo()
	dot := lipgloss.NewStyle().Foreground(t.CategoryColor(cat)).Render("●")
	left := t.CardKey.Render(d.issue.Key) + "  " +
		t.CardMeta.Render(string(d.issue.Type)) + "  " +
		dot + " " + lipgloss.NewStyle().Foreground(t.CategoryColor(cat)).Render(name)
	right := t.HelpDesc.Render("tab panels · esc back")
	inner := spread(left, right, d.width-2)
	return lipgloss.NewStyle().Background(t.P.BgSurface).Width(d.width).Render(" " + inner + " ")
}

// renderBento tiles the five panels: a two-column grid normally, or a single
// stacked column on a narrow terminal.
func (d *detailModel) renderBento() string {
	L := d.layout()
	if L.single {
		return lipgloss.JoinVertical(lipgloss.Left,
			panel(d.theme, panelOpts{title: "Description", body: d.descBody(L), width: L.leftW, height: L.descH, focused: d.focus == panelDescription}),
			panel(d.theme, panelOpts{title: "Properties", body: d.propertiesBody(L), width: L.leftW, height: L.propH, focused: d.focus == panelProperties}),
			panel(d.theme, panelOpts{title: "Agent", body: d.agentBody(L), width: L.leftW, height: L.agH, focused: d.focus == panelAgent}),
			panel(d.theme, panelOpts{title: "Relations & Files", body: d.relationsBody(L), width: L.leftW, height: L.relH, focused: d.focus == panelRelations}),
			panel(d.theme, panelOpts{title: "Activity", body: d.activityBody(L), width: L.leftW, height: L.actH, focused: d.focus == panelActivity}),
		)
	}
	left := lipgloss.JoinVertical(lipgloss.Left,
		panel(d.theme, panelOpts{title: "Description", body: d.descBody(L), width: L.leftW, height: L.descH, focused: d.focus == panelDescription}),
		panel(d.theme, panelOpts{title: "Activity", body: d.activityBody(L), width: L.leftW, height: L.actH, focused: d.focus == panelActivity}),
	)
	right := lipgloss.JoinVertical(lipgloss.Left,
		panel(d.theme, panelOpts{title: "Properties", body: d.propertiesBody(L), width: L.rightW, height: L.propH, focused: d.focus == panelProperties}),
		panel(d.theme, panelOpts{title: "Agent", body: d.agentBody(L), width: L.rightW, height: L.agH, focused: d.focus == panelAgent}),
		panel(d.theme, panelOpts{title: "Relations & Files", body: d.relationsBody(L), width: L.rightW, height: L.relH, focused: d.focus == panelRelations}),
	)
	gutter := strings.TrimRight(strings.Repeat(" \n", L.bodyH), "\n")
	return lipgloss.JoinHorizontal(lipgloss.Top, left, gutter, right)
}

// --- Description -------------------------------------------------------------

func (d *detailModel) descBody(L dlayout) string {
	t := d.theme
	iw := L.leftW - 4
	titleArea := twoLineTitle(t.Title.Width(iw).Render(d.issue.Title))

	if d.mode == dEditDesc {
		return titleArea + "\n\n" + d.desc.View()
	}
	editBtn := lipgloss.NewStyle().Width(iw).Align(lipgloss.Right).
		Render(button(t, iconEdit+" Edit", d.focus == panelDescription))
	return titleArea + "\n\n" + d.vp.View() + "\n" + editBtn
}

// twoLineTitle clamps a (possibly wrapped) title block to exactly two rows, so
// the description's scroll region below it never shifts.
func twoLineTitle(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) > 2 {
		lines = lines[:2]
	}
	for len(lines) < 2 {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// --- Activity ----------------------------------------------------------------

func (d *detailModel) activityBody(L dlayout) string {
	t := d.theme
	innerW := L.leftW - 4
	listH := L.actH - 2 - 1 // reserve the trailing Comment button
	if listH < 1 {
		listH = 1
	}

	var rows []string
	for _, c := range d.comments {
		who := t.Chip.Render(fmt.Sprintf("%-8s", truncate(c.Author, 8)))
		rows = append(rows, who+"  "+t.StatusText.Render(truncate(oneLine(c.Body), innerW-12)))
	}

	var window []string
	if len(rows) == 0 {
		window = []string{t.HelpDesc.Render("No activity yet — start the conversation.")}
	} else {
		maxStart := len(rows) - listH
		if maxStart < 0 {
			maxStart = 0
		}
		if d.commentScroll > maxStart {
			d.commentScroll = maxStart
		}
		start := maxStart - d.commentScroll
		if start < 0 {
			start = 0
		}
		end := start + listH
		if end > len(rows) {
			end = len(rows)
		}
		window = rows[start:end]
	}

	body := strings.Join(window, "\n")
	btn := lipgloss.NewStyle().Width(innerW).Align(lipgloss.Right).
		Render(button(t, iconAdd+" Comment", d.focus == panelActivity))
	// Pad the list region so the button sits on the last interior row.
	for lipgloss.Height(body) < listH {
		body += "\n"
	}
	return body + "\n" + btn
}

// --- Properties --------------------------------------------------------------

func (d *detailModel) propertiesBody(L dlayout) string {
	w := L.rightInner()
	focused := d.focus == panelProperties
	lines := make([]string, 0, len(propertyFields()))
	for _, f := range propertyFields() {
		lines = append(lines, d.fieldRow(f, w, focused && d.field == f))
	}
	return strings.Join(lines, "\n")
}

// cyclable reports whether a field changes with ←/→ (vs being edited via Enter).
func cyclable(f detailField) bool {
	switch f {
	case dfPoints, dfTags:
		return false
	}
	return true
}

// fieldRow renders one metadata row: a label and its value, with the active row
// wearing the iris label and either cycler chevrons (cyclable fields) or an Enter
// glyph (the edited ones). The value column is the same whether active or not.
func (d *detailModel) fieldRow(f detailField, w int, active bool) string {
	t := d.theme
	label, value, c := d.fieldDisplay(f)
	value = truncate(value, w-13)
	labelStyle := t.Label
	if active {
		labelStyle = lipgloss.NewStyle().Foreground(t.P.FocusGlow).Bold(true)
	}
	vs := lipgloss.NewStyle().Foreground(c)

	var control string
	switch {
	case active && cyclable(f):
		control = cycler(t, value, vs, true)
	case active:
		control = "  " + vs.Render(value) + "  " + lipgloss.NewStyle().Foreground(t.P.FocusGlow).Render("↵")
	default:
		control = "  " + vs.Render(value)
	}
	return labelStyle.Render(fmt.Sprintf("%-9s", label)) + control
}

// --- Agent -------------------------------------------------------------------

func (d *detailModel) agentBody(L dlayout) string {
	t := d.theme
	w := L.rightInner()
	focused := d.focus == panelAgent

	lines := []string{
		d.fieldRow(dfProvider, w, focused && d.agentSel == agProvider),
		d.fieldRow(dfModel, w, focused && d.agentSel == agModel),
		d.fieldRow(dfEffort, w, focused && d.agentSel == agEffort),
		d.toggleRow("Worktree", worktreeLabel(d.issue), w, focused && d.agentSel == agWorktree),
		"",
	}

	// Run is the primary action and sits alone on the first row; Discuss and
	// Schedule are the secondary pair beneath it.
	btn := -1
	if focused && d.agentSel >= agRun {
		btn = d.agentSel - agRun
	}
	row1Focus, row2Focus := -1, -1
	switch btn {
	case 0:
		row1Focus = 0
	case 1:
		row2Focus = 0
	case 2:
		row2Focus = 1
	}
	lines = append(lines,
		buttonRow(t, []string{iconRun + " Run"}, row1Focus),
		buttonRow(t, []string{iconDiscuss + " Discuss", iconClock + " Schedule"}, row2Focus),
	)

	if len(d.runs) > 0 {
		lines = append(lines, "", sectionTitle(t, "Runs"))
		for _, r := range d.runs[:min(2, len(d.runs))] {
			when := ""
			if r.StartedAt != nil {
				when = "  " + t.CardMeta.Render(r.StartedAt.Local().Format("Jan 2 15:04"))
			}
			rs := lipgloss.NewStyle().Foreground(t.RunStateColor(r.Status))
			lines = append(lines, fmt.Sprintf("v%-3d ", r.Version)+rs.Render(string(r.Status))+when)
		}
	}
	if len(d.schedules) > 0 {
		lines = append(lines, sectionTitle(t, "Schedule"))
		for _, sc := range d.schedules {
			next := ""
			if sc.NextRun != nil {
				next = " → " + sc.NextRun.Local().Format("Jan 2 15:04")
			}
			lines = append(lines, t.Chip.Render(iconClock+" "+sc.CronSpec)+t.HelpDesc.Render(next))
		}
	}
	return strings.Join(lines, "\n")
}

func worktreeLabel(iss core.Issue) string {
	if iss.WorktreeOn != nil && !*iss.WorktreeOn {
		return "off"
	}
	return "on"
}

// toggleRow renders a boolean row like a field row, cycling its label with ←/→.
func (d *detailModel) toggleRow(label, val string, w int, active bool) string {
	t := d.theme
	labelStyle := t.Label
	if active {
		labelStyle = lipgloss.NewStyle().Foreground(t.P.FocusGlow).Bold(true)
	}
	vs := lipgloss.NewStyle().Foreground(t.P.TextPrimary)
	control := "  " + vs.Render(val)
	if active {
		control = cycler(t, val, vs, true)
	}
	return labelStyle.Render(fmt.Sprintf("%-9s", label)) + control
}

// --- Relations & Files -------------------------------------------------------

func (d *detailModel) relationsBody(L dlayout) string {
	t := d.theme
	w := L.rightInner()
	focused := d.focus == panelRelations

	row := func(label, value string) string {
		return t.Label.Render(fmt.Sprintf("%-8s", label)) + " " + t.StatusText.Render(truncate(value, w-9))
	}
	parent := "—"
	if d.parent != nil {
		parent = d.parent.Key + " " + d.parent.Title
	}
	lines := []string{row("Epic", d.epicKey("—")), row("Parent", parent)}
	if len(d.children) > 0 {
		lines = append(lines, sectionTitle(t, fmt.Sprintf("Children · %d", len(d.children))))
		for _, c := range d.children[:min(2, len(d.children))] {
			lines = append(lines, t.CardKey.Render(c.Key)+" "+t.StatusText.Render(truncate(c.Title, w-len(c.Key)-1)))
		}
	}
	if len(d.links) > 0 {
		lines = append(lines, sectionTitle(t, "Links"))
		for _, l := range d.links[:min(2, len(d.links))] {
			lines = append(lines, t.CardMeta.Render(string(l.Type))+" "+t.CardKey.Render(l.Issue.Key))
		}
	}

	lines = append(lines, sectionTitle(t, fmt.Sprintf("Files · %d", len(d.attachments))))
	for i, a := range d.attachments {
		icon := iconClip
		if a.IsURL() {
			icon = iconLink
		}
		name := truncate(a.Filename, w-4)
		st := t.StatusText
		if focused && d.attachSel == i {
			st = lipgloss.NewStyle().Foreground(t.P.FocusGlow).Bold(true)
		}
		lines = append(lines, st.Render(icon+" "+name))
	}
	lines = append(lines, "", lipgloss.NewStyle().Width(w).Align(lipgloss.Right).
		Render(button(t, iconClip+" Attach", focused && d.attachSel == len(d.attachments))))
	return strings.Join(lines, "\n")
}

// --- footer ------------------------------------------------------------------

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
		hint = t.Label.Render(d.inputLabel()) + " " + d.input.View() + "   " + seg("enter", "save") + "  " + seg("esc", "cancel")
	case dEditDesc:
		hint = fitSegments([]string{seg("ctrl+s", "save"), seg("esc", "cancel")}, budget, t)
	default:
		hint = fitSegments(d.panelHints(seg), budget, t)
	}

	inner := spread(hint, right, d.width-2)
	return lipgloss.NewStyle().Background(t.P.BgSurface).Width(d.width).Render(" " + inner + " ")
}

// panelHints lists the footer segments for the focused panel — only the gestures
// that panel responds to, so the footer never advertises an inert action.
func (d *detailModel) panelHints(seg func(k, v string) string) []string {
	parts := []string{seg("tab", "panel")}
	switch d.focus {
	case panelDescription:
		parts = append(parts, seg("↑↓", "scroll"), seg("enter", "edit"))
	case panelProperties:
		parts = append(parts, seg("↑↓", "field"), seg("←→", "change"))
		if d.field == dfPoints || d.field == dfTags {
			parts = append(parts, seg("enter", "edit"))
		}
	case panelAgent:
		parts = append(parts, seg("↑↓", "row"), seg("←→", "change"), seg("enter", "act"))
	case panelRelations:
		parts = append(parts, seg("↑↓", "select"), seg("enter", "open/attach"))
	case panelActivity:
		parts = append(parts, seg("↑↓", "scroll"), seg("enter", "comment"))
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

// --- field display -----------------------------------------------------------

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

// oneLine flattens newlines so a multi-line comment body fits one activity row.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
