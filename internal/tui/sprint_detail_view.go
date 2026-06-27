package tui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// sprint_detail_view.go renders the Sprint bento. It composes the same panel(),
// button(), meter() and listRow() vocabulary the rest of the TUI is built from,
// so the Sprint view reads as one material with the board and the ticket detail.

// sdlayout holds the computed cell rectangles of the Sprint bento. The two
// columns each sum to bodyH so the grid tiles the body exactly, the way the
// ticket detail's dlayout does.
type sdlayout struct {
	bodyH                  int
	leftW, rightW          int
	single                 bool // narrow terminals stack every panel in one column
	goalH, burnH           int
	progH, breakH, issuesH int
}

func (d *sprintDetailModel) layout() sdlayout {
	w, h := d.width, d.height
	bodyH := h - 2 // title bar + footer
	if bodyH < 8 {
		bodyH = 8
	}

	// Below ~80 columns the two-column bento can't breathe, so stack the panels
	// in one full-width column; the view clips to height, keeping the top panels.
	if w < 80 {
		// Stacked, the interactive core (Goal, Progress, Issues) leads so it
		// survives a short terminal; the analytic Breakdown and the tall Burndown
		// follow and are the first to clip when there isn't room.
		goalH, progH, issuesH, breakH := 7, 11, 8, 8
		burnH := bodyH - goalH - progH - issuesH - breakH
		if burnH < 3 {
			burnH = 3
		}
		return sdlayout{bodyH: bodyH, leftW: w, single: true, goalH: goalH, progH: progH, breakH: breakH, burnH: burnH, issuesH: issuesH}
	}

	leftW := w * 60 / 100
	rightW := w - leftW - 1 // a 1-cell gutter between the columns
	if rightW < 30 {
		rightW = 30
		leftW = w - rightW - 1
	}
	if leftW < 32 {
		leftW = 32
	}

	// Left column: a short Goal band over the tall Burndown — the why above the
	// how-it's-going. The floor of 7 keeps the empty-goal invitation (four lines)
	// from clipping.
	goalH := bodyH * 26 / 100
	if goalH < 7 {
		goalH = 7
	}
	if goalH > 9 {
		goalH = 9
	}
	burnH := bodyH - goalH
	if burnH < 9 {
		burnH = 9
		goalH = bodyH - burnH
	}

	// Right column: Progress fixed at the height of its readout + buttons,
	// Breakdown at its four category rows, Issues taking the rest.
	progH := 11
	if progH > bodyH-14 {
		progH = bodyH - 14
	}
	if progH < 9 {
		progH = 9
	}
	breakH := 8
	if breakH > bodyH-progH-5 {
		breakH = bodyH - progH - 5
	}
	if breakH < 6 {
		breakH = 6
	}
	issuesH := bodyH - progH - breakH
	if issuesH < 5 {
		issuesH = 5
		breakH = bodyH - progH - issuesH
	}
	return sdlayout{bodyH: bodyH, leftW: leftW, rightW: rightW, goalH: goalH, burnH: burnH, progH: progH, breakH: breakH, issuesH: issuesH}
}

// rightInner is the interior text width of a right-column panel — or the single
// column's width when the layout has collapsed to one column.
func (L sdlayout) rightInner() int {
	if L.single {
		return L.leftW - 4
	}
	return L.rightW - 4
}

func (d *sprintDetailModel) View() string {
	title := d.renderTitleBar()
	body := d.renderBento()
	footer := d.renderFooter()
	content := lipgloss.JoinVertical(lipgloss.Left, title, body, footer)
	return fitHeight(content, d.height)
}

// renderTitleBar is the always-on identity row: the sprint icon, its name and
// live state on the left; the date window and day counter; how to leave on the
// right.
func (d *sprintDetailModel) renderTitleBar() string {
	t := d.theme
	c := t.SprintStateColor(d.sprint.State)
	dot := fgStyle(c).Render("●")
	right := t.HelpDesc.Render("tab panels · esc back")

	left := fgStyle(c).Render(iconSprints) + "  " + t.Title.Render(d.sprint.Name) + "   " +
		dot + " " + fgStyle(c).Render(string(d.sprint.State))
	// Append the date/day window only if the row still fits on one line, so a
	// narrow terminal drops it rather than wrapping the bar; clip as a last resort.
	if win := d.windowLabel(); win != "" {
		withWin := left + "   " + t.CardMeta.Render(win)
		if lipgloss.Width(withWin)+lipgloss.Width(right)+1 <= d.width-2 {
			left = withWin
		}
	}
	if budget := d.width - 2 - lipgloss.Width(right) - 1; budget > 0 {
		left = ansiClip(left, budget)
	}
	inner := spread(left, right, d.width-2)
	return lipgloss.NewStyle().Background(t.P.BgSurface).Width(d.width).Render(" " + inner + " ")
}

// windowLabel summarises the sprint window for the title bar: the date range,
// plus a "day N/M" counter while the sprint is running.
func (d *sprintDetailModel) windowLabel() string {
	loc := d.now.Location()
	win := formatWindow(d.sprint, loc)
	if !d.metrics.hasWindow {
		if win != "" {
			return win
		}
		return "no dates"
	}
	if d.sprint.State == core.SprintActive {
		return fmt.Sprintf("%s · day %d/%d", win, d.metrics.dayElapsed, d.metrics.dayTotal)
	}
	return win
}

// renderBento tiles the five panels: Goal over Burndown on the left, Progress /
// Breakdown / Issues on the right — or one stacked column on a narrow terminal.
func (d *sprintDetailModel) renderBento() string {
	t := d.theme
	L := d.layout()

	goalP := panel(t, panelOpts{title: "Goal", body: d.goalBody(L), width: L.leftW, height: L.goalH, focused: d.focus == spGoal})
	burnP := panel(t, panelOpts{title: "Burndown", body: d.burndownBody(L), width: L.leftW, height: L.burnH})

	rW := L.rightW
	if L.single {
		rW = L.leftW
	}
	progP := panel(t, panelOpts{title: "Progress", body: d.progressBody(L), width: rW, height: L.progH, focused: d.focus == spProgress})
	breakP := panel(t, panelOpts{title: "Breakdown", body: d.breakdownBody(L), width: rW, height: L.breakH})
	issuesP := panel(t, panelOpts{title: fmt.Sprintf("Issues · %d", len(d.issues)), body: d.issuesBody(L), width: rW, height: L.issuesH, focused: d.focus == spIssues})

	if L.single {
		return lipgloss.JoinVertical(lipgloss.Left, goalP, progP, issuesP, breakP, burnP)
	}
	left := lipgloss.JoinVertical(lipgloss.Left, goalP, burnP)
	right := lipgloss.JoinVertical(lipgloss.Left, progP, breakP, issuesP)
	gutter := strings.TrimRight(strings.Repeat(" \n", L.bodyH), "\n")
	return lipgloss.JoinHorizontal(lipgloss.Top, left, gutter, right)
}

// --- Goal --------------------------------------------------------------------

func (d *sprintDetailModel) goalBody(L sdlayout) string {
	t := d.theme
	iw := L.leftW - 4
	if d.mode == sdEditGoal {
		return d.goal.View()
	}
	var content string
	if goal := strings.TrimSpace(d.sprint.Goal); goal == "" {
		content = strings.Join([]string{
			t.HelpDesc.Render("No goal set."),
			"",
			fgStyle(t.P.TextSubtle).Render("A sprint goal names the one outcome this"),
			fgStyle(t.P.TextSubtle).Render("sprint is for. Press enter to write it."),
		}, "\n")
	} else {
		content = t.Title.Width(iw).Render(goal)
	}
	editBtn := lipgloss.NewStyle().Width(iw).Align(lipgloss.Right).Render(button(t, iconEdit+" Edit", d.focus == spGoal))
	return clampBlock(content, L.goalH-3) + "\n" + editBtn
}

// --- Burndown ----------------------------------------------------------------

func (d *sprintDetailModel) burndownBody(L sdlayout) string {
	return renderBurndown(d.theme, d.sprint, d.metrics, d.now.Location(), L.leftW-4, L.burnH-2)
}

// --- Progress ----------------------------------------------------------------

func (d *sprintDetailModel) progressBody(L sdlayout) string {
	t := d.theme
	w := L.rightInner()
	mt := d.metrics

	head := lipgloss.NewStyle().Foreground(t.P.FocusGlow).Bold(true).Render(fmt.Sprintf("%d%%", mt.percentComplete())) +
		" " + t.CardMeta.Render("complete · by "+mt.basisWord())

	btnFocus := -1
	if d.focus == spProgress {
		btnFocus = d.progressSel
	}

	lines := []string{
		head,
		meter(t, mt.basisDone, mt.basisTotal, w, t.P.Success),
		d.statRow("Points", fmt.Sprintf("%d / %d", mt.donePoints, mt.totalPoints), w),
		d.statRow("Issues", fmt.Sprintf("%d / %d", mt.doneIssues, mt.totalIssues), w),
		d.statRow("Timeline", d.timelineValue(), w),
		"",
		d.paceLine(),
		"",
		buttonRow(t, []string{sprintLifecycleLabel(d.sprint.State), iconClock + " Dates"}, btnFocus),
	}
	return strings.Join(lines, "\n")
}

func (d *sprintDetailModel) statRow(label, value string, w int) string {
	t := d.theme
	return spread(t.Label.Render(fmt.Sprintf("%-9s", label)), t.StatusText.Render(value), w)
}

func (d *sprintDetailModel) timelineValue() string {
	mt := d.metrics
	if !mt.hasWindow {
		return "no dates set"
	}
	switch d.sprint.State {
	case core.SprintCompleted:
		return "completed"
	case core.SprintActive:
		return fmt.Sprintf("day %d/%d · %d left", mt.dayElapsed, mt.dayTotal, mt.daysLeft)
	default:
		return fmt.Sprintf("%d days", mt.dayTotal)
	}
}

// paceLine states the sprint's health in words, coloured to match: the burndown
// shows the trend, this names it.
func (d *sprintDetailModel) paceLine() string {
	t := d.theme
	mt := d.metrics
	switch mt.pace {
	case paceAhead:
		return fgStyle(t.P.Success).Render("● Ahead of schedule") + " " + t.CardMeta.Render(fmt.Sprintf("· %d %s under", mt.paceDelta, mt.basisUnit(mt.paceDelta)))
	case paceBehind:
		return fgStyle(t.P.Warning).Render("● Behind schedule") + " " + t.CardMeta.Render(fmt.Sprintf("· %d %s over", mt.paceDelta, mt.basisUnit(mt.paceDelta)))
	case paceOnTrack:
		return fgStyle(t.P.Info).Render("● On track")
	default:
		switch {
		case d.sprint.State == core.SprintCompleted:
			return fgStyle(t.P.Success).Render("● Completed")
		case d.sprint.State == core.SprintFuture:
			return t.HelpDesc.Render("○ Not started")
		case mt.basisTotal == 0:
			return t.HelpDesc.Render("○ No work yet")
		default: // active, but no dated window to measure against
			return fgStyle(t.P.Focus).Render("● Active")
		}
	}
}

// --- Breakdown ---------------------------------------------------------------

func (d *sprintDetailModel) breakdownBody(L sdlayout) string {
	t := d.theme
	w := L.rightInner()
	mt := d.metrics
	barW := w - 22
	if barW < 6 {
		barW = 6
	}

	lines := make([]string, 0, len(core.StatusCategories())+2)
	for _, cat := range core.StatusCategories() {
		cs := mt.byCat[cat]
		val, tot := cs.points, mt.totalPoints
		meta := fmt.Sprintf("%d·%dp", cs.count, cs.points)
		if !mt.pointsBased {
			val, tot = cs.count, mt.totalIssues
			meta = fmt.Sprintf("%d", cs.count)
		}
		label := fgStyle(t.CategoryColor(cat)).Render(fmt.Sprintf("%-11s", categoryLabel(cat)))
		lines = append(lines, label+" "+meter(t, val, tot, barW, t.CategoryColor(cat))+" "+t.CardMeta.Render(meta))
	}
	if mix := typeMix(d.issues); mix != "" {
		lines = append(lines, "", t.CardMeta.Render(mix))
	}
	return strings.Join(lines, "\n")
}

// --- Issues ------------------------------------------------------------------

func (d *sprintDetailModel) issuesBody(L sdlayout) string {
	t := d.theme
	w := L.rightInner()
	if len(d.issues) == 0 {
		return strings.Join([]string{
			t.HelpDesc.Render("No issues in this sprint yet."),
			"",
			fgStyle(t.P.TextSubtle).Render("Add them from the Sprints list."),
		}, "\n")
	}
	innerH := L.issuesH - 2
	if innerH < 1 {
		innerH = 1
	}
	start, end := scrollWindow(d.issueSel, len(d.issues), innerH)
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		lines = append(lines, d.issueRow(d.issues[i], d.focus == spIssues && i == d.issueSel, w))
	}
	return strings.Join(lines, "\n")
}

// issueRow renders one of the sprint's issues as a full-width selection row —
// status dot, key, title and trailing points — the same idiom as the board and
// backlog lists, so an issue reads identically wherever it appears.
func (d *sprintDetailModel) issueRow(iss core.Issue, selected bool, w int) string {
	t := d.theme
	cat := categoryOf(iss, d.statuses)
	titleW := w - 16
	if titleW < 6 {
		titleW = 6
	}
	left := []cell{
		cFg("● ", t.CategoryColor(cat)),
		cKey(fmt.Sprintf("%-8s ", iss.Key), t.P.Focus),
		cText(truncate(iss.Title, titleW)),
	}
	right := []cell{cText("")}
	if iss.StoryPoints != nil {
		right = []cell{cFg(fmt.Sprintf("%dpt", *iss.StoryPoints), t.P.TextSubtle)}
	}
	return listRow(t, w, selected, left, right)
}

// --- footer ------------------------------------------------------------------

func (d *sprintDetailModel) renderFooter() string {
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
	case sdInput:
		hint = t.Label.Render("Dates:") + " " + d.input.View() + "   " + seg("enter", "save") + "  " + seg("esc", "cancel")
	case sdEditGoal:
		hint = fitSegments([]string{seg("ctrl+s", "save"), seg("esc", "cancel")}, budget, t)
	default:
		hint = fitSegments(d.panelHints(seg), budget, t)
	}

	inner := spread(hint, right, d.width-2)
	return lipgloss.NewStyle().Background(t.P.BgSurface).Width(d.width).Render(" " + inner + " ")
}

// panelHints lists the footer segments for the focused panel — only the gestures
// that panel responds to, so the footer never advertises an inert action.
func (d *sprintDetailModel) panelHints(seg func(k, v string) string) []string {
	parts := []string{seg("tab", "panel")}
	switch d.focus {
	case spGoal:
		parts = append(parts, seg("enter", "edit goal"))
	case spProgress:
		action := sprintCycleVerb(d.sprint.State)
		if d.progressSel == spbDates {
			action = "edit dates"
		}
		parts = append(parts, seg("←→", "select"), seg("enter", action))
	case spIssues:
		parts = append(parts, seg("↑↓", "select"), seg("enter", "open"))
	}
	return append(parts, seg("esc", "back"))
}

// --- small render helpers ----------------------------------------------------

func fgStyle(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

// clampBlock clips s to n rows and pads it up to n, so a panel body that
// reserves a trailing row (e.g. the Goal's Edit button) lands it predictably.
func clampBlock(s string, n int) string {
	if n < 0 {
		n = 0
	}
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	for len(lines) < n {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// categoryLabel is the display name for a board category in the Breakdown panel.
func categoryLabel(c core.StatusCategory) string {
	switch c {
	case core.CategoryInProgress:
		return "In Progress"
	case core.CategoryReview:
		return "In Review"
	case core.CategoryDone:
		return "Done"
	default:
		return "To Do"
	}
}

// typeMix summarises the sprint's issue types, e.g. "3 story · 2 task · 1 bug",
// listing only the types present, in the canonical order.
func typeMix(issues []core.Issue) string {
	counts := make(map[core.IssueType]int, len(core.IssueTypes()))
	for _, iss := range issues {
		counts[iss.Type]++
	}
	var parts []string
	for _, ty := range core.IssueTypes() {
		if n := counts[ty]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, string(ty)))
		}
	}
	return strings.Join(parts, " · ")
}

// formatWindow renders a sprint's date window in loc, tolerating either bound
// being unset. It mirrors sprintDates but is location-aware for determinism.
func formatWindow(sp core.Sprint, loc *time.Location) string {
	const f = "Jan 2"
	switch {
	case sp.StartAt != nil && sp.EndAt != nil:
		return sp.StartAt.In(loc).Format(f) + " – " + sp.EndAt.In(loc).Format(f)
	case sp.StartAt != nil:
		return "from " + sp.StartAt.In(loc).Format(f)
	case sp.EndAt != nil:
		return "until " + sp.EndAt.In(loc).Format(f)
	}
	return ""
}
