package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/store"
	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

// sprint_detail.go is the full-screen Sprint view: a bento of panels — Goal,
// Burndown, Progress, Breakdown and Issues — opened from a sprint header in the
// Sprints list, mirroring the ticket detail's architecture (a self-contained
// sub-model with an Update returning (cmd, back) and a synchronous reload). It
// is the sprint's command center: the one outcome it's for, how the work is
// burning down, where the points sit, and a way into any of its issues.

// sprintPanel is one focusable region. Only the three that respond to keys take
// focus; Burndown and Breakdown are always-on readouts, so TAB never lands on a
// panel with nothing to do.
type sprintPanel int

const (
	spGoal sprintPanel = iota
	spProgress
	spIssues
	spPanelCount
)

type sprintDetailMode int

const (
	sdViewing sprintDetailMode = iota
	sdEditGoal
	sdInput
)

// The Progress panel's two actions, indexed by progressSel.
const (
	spbLifecycle = iota // start / finish / reopen, per the sprint's state
	spbDates            // edit the start/end window
	spbActionCount
)

// pace classifies the active sprint against its ideal burndown at today.
type pace int

const (
	paceUnknown pace = iota
	paceAhead
	paceOnTrack
	paceBehind
)

// catStat is the issue count and story-point sum for one status category.
type catStat struct {
	count  int
	points int
}

// burndownPoint is one day of the sprint: the ideal remaining work (the
// straight guideline) and the reconstructed actual remaining. hasActual is
// false for days after today, where there is no history to draw.
type burndownPoint struct {
	day       int
	ideal     float64
	actual    float64
	hasActual bool
}

// sprintMetrics is everything the view's readouts and chart derive from a
// sprint and its issues — computed once per reload by computeSprintMetrics so
// the render is pure formatting. "basis" is the burndown unit: story points
// when any issue is estimated, else a headcount of issues, so an unestimated
// sprint still gets a meaningful chart instead of a flat line.
type sprintMetrics struct {
	totalIssues int
	doneIssues  int
	totalPoints int
	donePoints  int

	byCat map[core.StatusCategory]catStat

	pointsBased bool
	basisTotal  int
	basisDone   int

	started    bool
	hasWindow  bool
	dayTotal   int
	dayElapsed int
	daysLeft   int

	series    []burndownPoint
	pace      pace
	paceDelta int // |actual − ideal| at today, in basis units
}

func (mt sprintMetrics) percentComplete() int {
	if mt.basisTotal == 0 {
		return 0
	}
	return int(math.Round(float64(mt.basisDone) / float64(mt.basisTotal) * 100))
}

// basisWord is the plural unit label for the headline figures ("by pts" /
// "by issues"), matching the burndown.
func (mt sprintMetrics) basisWord() string {
	if mt.pointsBased {
		return "pts"
	}
	return "issues"
}

// basisUnit is the unit for a specific count n, pluralised — so a pace delta
// reads "1 issue over", not "1 issues over".
func (mt sprintMetrics) basisUnit(n int) string {
	switch {
	case mt.pointsBased && n == 1:
		return "pt"
	case mt.pointsBased:
		return "pts"
	case n == 1:
		return "issue"
	default:
		return "issues"
	}
}

// categoryOf resolves an issue's board category via the project's statuses,
// defaulting to To Do when a status is missing (e.g. mid-migration data).
func categoryOf(iss core.Issue, statuses map[int64]core.Status) core.StatusCategory {
	if s, ok := statuses[iss.StatusID]; ok {
		return s.Category
	}
	return core.CategoryTodo
}

// startOfDay truncates a time to midnight in its own location, the granularity
// the burndown's day buckets and the "day N of M" counter work in.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// daysBetween counts whole days from a to b, rounding so a daylight-saving
// boundary (a 23- or 25-hour day) doesn't drop or double-count a bucket.
func daysBetween(a, b time.Time) int {
	return int(math.Round(b.Sub(a).Hours() / 24))
}

// computeSprintMetrics reduces a sprint and its issues to the figures the view
// renders. now (and its location) is the reference "today": injected so the
// reconstruction and the day counters are deterministic under test.
func computeSprintMetrics(sp core.Sprint, issues []core.Issue, statuses map[int64]core.Status, now time.Time) sprintMetrics {
	mt := sprintMetrics{byCat: make(map[core.StatusCategory]catStat, len(core.StatusCategories()))}

	for _, iss := range issues {
		pts := 0
		if iss.StoryPoints != nil {
			pts = *iss.StoryPoints
			mt.pointsBased = true
		}
		cat := categoryOf(iss, statuses)
		cs := mt.byCat[cat]
		cs.count++
		cs.points += pts
		mt.byCat[cat] = cs

		mt.totalIssues++
		mt.totalPoints += pts
		if cat == core.CategoryDone {
			mt.doneIssues++
			mt.donePoints += pts
		}
	}

	if mt.pointsBased {
		mt.basisTotal, mt.basisDone = mt.totalPoints, mt.donePoints
	} else {
		mt.basisTotal, mt.basisDone = mt.totalIssues, mt.doneIssues
	}

	mt.started = sp.State == core.SprintActive || sp.State == core.SprintCompleted

	if sp.StartAt != nil && sp.EndAt != nil {
		loc := now.Location()
		start := startOfDay(sp.StartAt.In(loc))
		end := startOfDay(sp.EndAt.In(loc))
		mt.hasWindow = true
		mt.dayTotal = imax(daysBetween(start, end), 1)
		mt.dayElapsed = iclamp(daysBetween(start, startOfDay(now)), 0, mt.dayTotal)
		mt.daysLeft = mt.dayTotal - mt.dayElapsed
		mt.series = buildBurndown(issues, statuses, mt, start, loc)
		mt.pace, mt.paceDelta = computePace(sp, mt)
	}
	return mt
}

// buildBurndown reconstructs the remaining-work line. With no per-day history
// stored, a done issue is taken to have burned down on its last-updated day
// (the move to Done is its last touch in the common case) — an honest best
// effort that the straight ideal line is never conflated with. Remaining on
// day d is the scope minus everything completed on or before d.
func buildBurndown(issues []core.Issue, statuses map[int64]core.Status, mt sprintMetrics, start time.Time, loc *time.Location) []burndownPoint {
	type completion struct{ day, basis int }
	var done []completion
	for _, iss := range issues {
		if categoryOf(iss, statuses) != core.CategoryDone {
			continue
		}
		basis := 1
		if mt.pointsBased {
			basis = 0
			if iss.StoryPoints != nil {
				basis = *iss.StoryPoints
			}
		}
		day := iclamp(daysBetween(start, startOfDay(iss.UpdatedAt.In(loc))), 0, mt.dayTotal)
		done = append(done, completion{day: day, basis: basis})
	}

	series := make([]burndownPoint, 0, mt.dayTotal+1)
	for d := 0; d <= mt.dayTotal; d++ {
		completed := 0
		for _, c := range done {
			if c.day <= d {
				completed += c.basis
			}
		}
		series = append(series, burndownPoint{
			day:       d,
			ideal:     float64(mt.basisTotal) * float64(mt.dayTotal-d) / float64(mt.dayTotal),
			actual:    float64(mt.basisTotal - completed),
			hasActual: d <= mt.dayElapsed,
		})
	}
	return series
}

// computePace reads an active sprint's health off the ideal line at today: at
// or below it is on track / ahead, above it is behind. It is meaningful only
// for a running, dated, non-empty sprint; everything else is unknown.
func computePace(sp core.Sprint, mt sprintMetrics) (pace, int) {
	if sp.State != core.SprintActive || !mt.hasWindow || mt.basisTotal == 0 {
		return paceUnknown, 0
	}
	ideal := float64(mt.basisTotal) * float64(mt.dayTotal-mt.dayElapsed) / float64(mt.dayTotal)
	actual := float64(mt.basisTotal - mt.basisDone)
	delta := actual - ideal
	mag := int(math.Round(math.Abs(delta)))
	switch {
	case delta <= -0.5:
		return paceAhead, mag
	case delta >= 0.5:
		return paceBehind, mag
	default:
		return paceOnTrack, 0
	}
}

// sprintDetailModel is the full-screen Sprint view. Like detailModel it owns its
// store handle and persists edits immediately, reloading so it stays consistent
// with concurrent agent changes seen over MCP.
type sprintDetailModel struct {
	store *store.Store
	theme theme.Theme

	sprintID int64
	sprint   core.Sprint
	statuses map[int64]core.Status
	issues   []core.Issue
	metrics  sprintMetrics

	mode        sprintDetailMode
	focus       sprintPanel
	issueSel    int
	progressSel int

	goal      textarea.Model
	input     textinput.Model
	inputKind sprintInputKind

	// clock is the seam for "now"; production uses time.Now, tests pin it so the
	// day counters and the reconstructed burndown render deterministically.
	clock func() time.Time
	now   time.Time

	width, height int
	err           string
	notice        string
}

type sprintInputKind int

const (
	siDates sprintInputKind = iota
)

// newSprintDetail builds the view over a sprint and loads its data synchronously,
// the same shape as newDetail so the parent opens it identically.
func newSprintDetail(st *store.Store, th theme.Theme, sprintID int64, w, h int) *sprintDetailModel {
	d := &sprintDetailModel{store: st, theme: th, sprintID: sprintID, clock: time.Now}
	d.setSize(w, h)
	d.reload()
	return d
}

func (d *sprintDetailModel) setSize(w, h int) { d.width, d.height = w, h }

// reload re-reads the sprint, its project's statuses and its issues, then
// recomputes the metrics. Called on open, after every edit, and on any store
// event while the view is up.
func (d *sprintDetailModel) reload() {
	sp, err := d.store.GetSprint(d.sprintID)
	if err != nil {
		d.err = err.Error()
		return
	}
	d.sprint = sp

	statusList, _ := d.store.ListStatuses(sp.ProjectID)
	d.statuses = make(map[int64]core.Status, len(statusList))
	for _, s := range statusList {
		d.statuses[s.ID] = s
	}

	id := sp.ID
	d.issues, _ = d.store.ListIssues(store.IssueFilter{ProjectID: sp.ProjectID, SprintID: &id})
	if d.issueSel >= len(d.issues) {
		d.issueSel = len(d.issues) - 1
	}
	if d.issueSel < 0 {
		d.issueSel = 0
	}
	d.recompute()
}

// recompute refreshes the derived metrics from the current data and clock. It
// is split from reload so tests can stage in-memory data (with fixed
// timestamps) and recompute without touching the store.
func (d *sprintDetailModel) recompute() {
	d.now = d.clock()
	d.metrics = computeSprintMetrics(d.sprint, d.issues, d.statuses, d.now)
}

func (d *sprintDetailModel) selectedIssue() (core.Issue, bool) {
	if d.issueSel < 0 || d.issueSel >= len(d.issues) {
		return core.Issue{}, false
	}
	return d.issues[d.issueSel], true
}

// Update handles a message while the Sprint view is focused, returning a command
// and whether to return to the Sprints list.
func (d *sprintDetailModel) Update(msg tea.Msg) (tea.Cmd, bool) {
	switch d.mode {
	case sdEditGoal:
		return d.updateEditGoal(msg)
	case sdInput:
		return d.updateInput(msg)
	default:
		return d.updateViewing(msg)
	}
}

func (d *sprintDetailModel) updateViewing(msg tea.Msg) (tea.Cmd, bool) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil, false
	}
	d.notice = ""
	switch k.String() {
	case "esc", "q":
		return nil, true
	case "tab":
		d.focusPanel(+1)
		return nil, false
	case "shift+tab":
		d.focusPanel(-1)
		return nil, false
	}
	switch d.focus {
	case spGoal:
		return d.keyGoal(k)
	case spProgress:
		return d.keyProgress(k)
	case spIssues:
		return d.keyIssues(k)
	}
	return nil, false
}

// focusPanel moves focus and lands the new panel's cursor somewhere valid.
func (d *sprintDetailModel) focusPanel(dir int) {
	d.focus = sprintPanel(wrap(int(d.focus)+dir, int(spPanelCount)))
	if d.focus == spProgress && (d.progressSel < 0 || d.progressSel >= spbActionCount) {
		d.progressSel = 0
	}
}

func (d *sprintDetailModel) keyGoal(k tea.KeyPressMsg) (tea.Cmd, bool) {
	switch k.String() {
	case "enter", "e":
		return d.startEditGoal(), false
	}
	return nil, false
}

func (d *sprintDetailModel) keyProgress(k tea.KeyPressMsg) (tea.Cmd, bool) {
	switch k.String() {
	case "left", "h", "up", "k":
		d.progressSel = wrap(d.progressSel-1, spbActionCount)
	case "right", "l", "down", "j":
		d.progressSel = wrap(d.progressSel+1, spbActionCount)
	case "enter":
		switch d.progressSel {
		case spbLifecycle:
			d.cycleLifecycle()
		case spbDates:
			return d.startDateInput(), false
		}
	}
	return nil, false
}

func (d *sprintDetailModel) keyIssues(k tea.KeyPressMsg) (tea.Cmd, bool) {
	switch k.String() {
	case "up", "k":
		if d.issueSel > 0 {
			d.issueSel--
		}
	case "down", "j":
		if d.issueSel < len(d.issues)-1 {
			d.issueSel++
		}
	case "enter", "o":
		if iss, ok := d.selectedIssue(); ok {
			id := iss.ID
			// Hand the parent the issue to open; it owns the run manager and
			// scheduler the ticket detail needs, and remembers this sprint view so
			// leaving the issue returns here rather than to the list.
			return func() tea.Msg { return openIssueDetailMsg{issueID: id} }, false
		}
	}
	return nil, false
}

// cycleLifecycle advances the sprint through its lifecycle, going through
// StartSprint/CompleteSprint so the SCRUM rules hold (one active sprint per
// project; a finished sprint's unfinished issues roll back to the backlog).
func (d *sprintDetailModel) cycleLifecycle() {
	var (
		err  error
		verb string
	)
	switch d.sprint.State {
	case core.SprintFuture:
		err, verb = d.store.StartSprint(d.sprint.ID), "started"
	case core.SprintActive:
		err, verb = d.store.CompleteSprint(d.sprint.ID), "completed"
	default: // completed → reopen for re-planning
		sp := d.sprint
		sp.State = core.SprintFuture
		err, verb = d.store.UpdateSprint(sp), "reopened"
	}
	if err != nil {
		d.err = err.Error()
		return
	}
	d.err, d.notice = "", verb+" "+d.sprint.Name
	d.reload()
}

func (d *sprintDetailModel) startEditGoal() tea.Cmd {
	L := d.layout()
	ta := textarea.New()
	ta.SetWidth(L.leftW - 4)
	ta.SetHeight(imax(1, L.goalH-3))
	ta.SetValue(d.sprint.Goal)
	d.goal = ta
	d.mode = sdEditGoal
	return d.goal.Focus()
}

func (d *sprintDetailModel) updateEditGoal(msg tea.Msg) (tea.Cmd, bool) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			d.mode = sdViewing
			return nil, false
		case "ctrl+s":
			d.sprint.Goal = strings.TrimSpace(d.goal.Value())
			if err := d.store.UpdateSprint(d.sprint); err != nil {
				d.err = err.Error()
			} else {
				d.err = ""
			}
			d.mode = sdViewing
			d.reload()
			return nil, false
		}
	}
	var cmd tea.Cmd
	d.goal, cmd = d.goal.Update(msg)
	return cmd, false
}

func (d *sprintDetailModel) startDateInput() tea.Cmd {
	ti := textinput.New()
	ti.SetWidth(44)
	ti.Placeholder = "2026-06-20 2026-07-04  (start end · blank clears)"
	ti.SetValue(currentDatesString(d.sprint, d.now.Location()))
	d.input = ti
	d.inputKind = siDates
	d.mode = sdInput
	return d.input.Focus()
}

func (d *sprintDetailModel) updateInput(msg tea.Msg) (tea.Cmd, bool) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			d.mode = sdViewing
			return nil, false
		case "enter":
			d.submitDates(strings.TrimSpace(d.input.Value()))
			d.mode = sdViewing
			d.reload()
			return nil, false
		}
	}
	var cmd tea.Cmd
	d.input, cmd = d.input.Update(msg)
	return cmd, false
}

// submitDates parses the "start end" field and persists the window. An invalid
// date or an end-before-start range (rejected by Sprint.Validate) surfaces in
// the footer and leaves the stored dates untouched.
func (d *sprintDetailModel) submitDates(value string) {
	start, end, err := parseDateRange(value, d.now.Location())
	if err != nil {
		d.err = err.Error()
		return
	}
	prevStart, prevEnd := d.sprint.StartAt, d.sprint.EndAt
	d.sprint.StartAt, d.sprint.EndAt = start, end
	if err := d.store.UpdateSprint(d.sprint); err != nil {
		d.sprint.StartAt, d.sprint.EndAt = prevStart, prevEnd
		d.err = err.Error()
		return
	}
	d.err = ""
}

// parseDateRange reads "start [end]" YYYY-MM-DD dates in loc. An empty string
// clears both bounds; one date sets only the start.
func parseDateRange(s string, loc *time.Location) (start, end *time.Time, err error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil, nil, nil
	}
	if len(fields) > 2 {
		return nil, nil, fmt.Errorf("enter one or two dates: start [end]")
	}
	parse := func(f string) (*time.Time, error) {
		tm, perr := time.ParseInLocation("2006-01-02", f, loc)
		if perr != nil {
			return nil, fmt.Errorf("dates must look like 2026-06-20")
		}
		return &tm, nil
	}
	if start, err = parse(fields[0]); err != nil {
		return nil, nil, err
	}
	if len(fields) == 2 {
		if end, err = parse(fields[1]); err != nil {
			return nil, nil, err
		}
	}
	return start, end, nil
}

func currentDatesString(sp core.Sprint, loc *time.Location) string {
	var parts []string
	if sp.StartAt != nil {
		parts = append(parts, sp.StartAt.In(loc).Format("2006-01-02"))
	}
	if sp.EndAt != nil {
		parts = append(parts, sp.EndAt.In(loc).Format("2006-01-02"))
	}
	return strings.Join(parts, " ")
}

// sprintLifecycleLabel is the Progress action button's label, naming what the
// next press does given the sprint's state. The single-word form for the footer
// hint is the shared sprintCycleVerb (sprints.go), so the Sprints list and this
// view always agree on what s/start/finish/reopen does.
func sprintLifecycleLabel(s core.SprintState) string {
	switch s {
	case core.SprintFuture:
		return "Start sprint"
	case core.SprintActive:
		return "Finish sprint"
	default:
		return "Reopen sprint"
	}
}
