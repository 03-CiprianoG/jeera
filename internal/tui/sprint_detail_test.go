package tui

import (
	"testing"
	"time"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/store"
	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

// --- metrics -----------------------------------------------------------------

func TestComputeSprintMetricsPointsBasis(t *testing.T) {
	sp := core.Sprint{Name: "S", State: core.SprintActive}
	issues := []core.Issue{
		{ID: 1, StatusID: 4, StoryPoints: ptr(5)}, // done
		{ID: 2, StatusID: 3, StoryPoints: ptr(5)}, // review
		{ID: 3, StatusID: 2, StoryPoints: ptr(3)}, // in progress
		{ID: 4, StatusID: 1, StoryPoints: ptr(3)}, // todo
	}
	mt := computeSprintMetrics(sp, issues, testStatuses(), mustDayUTC(t, "2026-06-27"))

	if !mt.pointsBased {
		t.Error("any estimate should select the points basis")
	}
	if mt.totalPoints != 16 || mt.donePoints != 5 {
		t.Errorf("points: total=%d done=%d want 16/5", mt.totalPoints, mt.donePoints)
	}
	if mt.totalIssues != 4 || mt.doneIssues != 1 {
		t.Errorf("issues: total=%d done=%d want 4/1", mt.totalIssues, mt.doneIssues)
	}
	if got := mt.percentComplete(); got != 31 { // round(5/16)
		t.Errorf("percent=%d want 31", got)
	}
	if mt.byCat[core.CategoryReview].points != 5 || mt.byCat[core.CategoryReview].count != 1 {
		t.Errorf("review cat = %+v", mt.byCat[core.CategoryReview])
	}
}

func TestComputeSprintMetricsIssueFallback(t *testing.T) {
	sp := core.Sprint{Name: "S", State: core.SprintActive}
	issues := []core.Issue{
		{ID: 1, StatusID: 4}, // done, unestimated
		{ID: 2, StatusID: 2},
		{ID: 3, StatusID: 1},
	}
	mt := computeSprintMetrics(sp, issues, testStatuses(), mustDayUTC(t, "2026-06-27"))

	if mt.pointsBased {
		t.Error("with no estimates the basis should fall back to issue count")
	}
	if mt.basisTotal != 3 || mt.basisDone != 1 {
		t.Errorf("basis total/done = %d/%d want 3/1", mt.basisTotal, mt.basisDone)
	}
	if got := mt.percentComplete(); got != 33 {
		t.Errorf("percent=%d want 33", got)
	}
	if mt.basisWord() != "issues" {
		t.Errorf("basisWord=%q want issues", mt.basisWord())
	}
}

func TestComputeSprintMetricsWindowAndPace(t *testing.T) {
	start := mustDayUTC(t, "2026-06-20")
	end := mustDayUTC(t, "2026-07-04") // 14-day window
	now := mustDayUTC(t, "2026-06-27") // day 7 — exactly half
	sp := core.Sprint{Name: "S", State: core.SprintActive, StartAt: &start, EndAt: &end}

	// 14 points of scope, so the ideal at the half-way day is 7 remaining.
	base := func(donePts int) []core.Issue {
		return []core.Issue{
			doneIssue(1, donePts, "2026-06-24"),
			{ID: 2, StatusID: 1, StoryPoints: ptr(14 - donePts)},
		}
	}

	mt := computeSprintMetrics(sp, base(2), testStatuses(), now) // 12 remaining > 7 ideal
	if mt.dayTotal != 14 || mt.dayElapsed != 7 || mt.daysLeft != 7 {
		t.Errorf("window days: total=%d elapsed=%d left=%d want 14/7/7", mt.dayTotal, mt.dayElapsed, mt.daysLeft)
	}
	if mt.pace != paceBehind || mt.paceDelta != 5 {
		t.Errorf("pace=%v delta=%d want behind/5", mt.pace, mt.paceDelta)
	}

	if mt := computeSprintMetrics(sp, base(10), testStatuses(), now); mt.pace != paceAhead || mt.paceDelta != 3 {
		t.Errorf("ahead case: pace=%v delta=%d want ahead/3", mt.pace, mt.paceDelta)
	}
	if mt := computeSprintMetrics(sp, base(7), testStatuses(), now); mt.pace != paceOnTrack {
		t.Errorf("on-track case: pace=%v want onTrack", mt.pace)
	}
}

func TestBuildBurndownSeriesEndpoints(t *testing.T) {
	start := mustDayUTC(t, "2026-06-20")
	end := mustDayUTC(t, "2026-07-04")
	now := mustDayUTC(t, "2026-06-27")
	sp := core.Sprint{Name: "S", State: core.SprintActive, StartAt: &start, EndAt: &end}
	issues := []core.Issue{
		doneIssue(1, 5, "2026-06-22"),
		doneIssue(2, 3, "2026-06-25"),
		{ID: 3, StatusID: 2, StoryPoints: ptr(8)},
	}
	mt := computeSprintMetrics(sp, issues, testStatuses(), now)

	if len(mt.series) != mt.dayTotal+1 {
		t.Fatalf("series has %d points, want %d", len(mt.series), mt.dayTotal+1)
	}
	first, last := mt.series[0], mt.series[len(mt.series)-1]
	if first.ideal != 16 {
		t.Errorf("ideal should start at the full scope 16, got %v", first.ideal)
	}
	if last.ideal != 0 {
		t.Errorf("ideal should end at 0, got %v", last.ideal)
	}
	if first.actual != 16 {
		t.Errorf("nothing is done on day 0, actual should be 16, got %v", first.actual)
	}
	// Day 7 (today) is the last point with actual data; later days are the future.
	if !mt.series[mt.dayElapsed].hasActual || mt.series[mt.dayElapsed+1].hasActual {
		t.Error("actual data should stop at today")
	}
	// After both completions (by day 5) remaining is 16 − 8 = 8.
	if got := mt.series[5].actual; got != 8 {
		t.Errorf("remaining after both completions = %v, want 8", got)
	}
}

// --- lifecycle / edits (store-backed) ----------------------------------------

func newSprintDetailStore(t *testing.T) (*sprintDetailModel, *store.Store, core.Project, core.Sprint) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/jeera.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, _ := st.CreateProject(core.Project{Name: "Jeera", KeyPrefix: "JEE", RepoPath: "/tmp/jeera"})
	sp, _ := st.CreateSprint(core.Sprint{ProjectID: p.ID, Name: "Sprint 1", State: core.SprintFuture})
	d := newSprintDetail(st, theme.New(), sp.ID, 100, 30)
	return d, st, p, sp
}

func TestSprintDetailLifecycle(t *testing.T) {
	d, st, p, sp := newSprintDetailStore(t)
	statuses, _ := st.ListStatuses(p.ID)
	todo, done := statuses[0], statuses[3]

	todoIss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "unfinished", StatusID: todo.ID})
	doneIss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "shipped", StatusID: done.ID})
	st.AddIssueToSprint(todoIss.ID, &sp.ID)
	st.AddIssueToSprint(doneIss.ID, &sp.ID)
	d.reload()

	// future → active
	d.cycleLifecycle()
	if d.sprint.State != core.SprintActive {
		t.Fatalf("start: state=%q want active", d.sprint.State)
	}

	// active → completed: the unfinished issue rolls back to the backlog, the done
	// one stays attached so the record of what shipped survives.
	d.cycleLifecycle()
	if d.sprint.State != core.SprintCompleted {
		t.Fatalf("finish: state=%q want completed", d.sprint.State)
	}
	rolled, _ := st.GetIssue(todoIss.ID)
	if rolled.SprintID != nil {
		t.Errorf("unfinished issue should return to backlog, sprint=%v", rolled.SprintID)
	}
	kept, _ := st.GetIssue(doneIss.ID)
	if kept.SprintID == nil || *kept.SprintID != sp.ID {
		t.Errorf("done issue should stay in the completed sprint, sprint=%v", kept.SprintID)
	}

	// completed → reopened (future)
	d.cycleLifecycle()
	if d.sprint.State != core.SprintFuture {
		t.Fatalf("reopen: state=%q want future", d.sprint.State)
	}
}

func TestSprintDetailEditGoalSaves(t *testing.T) {
	d, st, _, sp := newSprintDetailStore(t)
	d.startEditGoal()
	d.goal.SetValue("Ship a world-class sprint view")
	if _, back := d.updateEditGoal(keyPress2("ctrl+s")); back {
		t.Error("ctrl+s should not leave the view")
	}
	reloaded, _ := st.GetSprint(sp.ID)
	if reloaded.Goal != "Ship a world-class sprint view" {
		t.Errorf("goal not saved: %q", reloaded.Goal)
	}
}

func TestSprintDetailEditDates(t *testing.T) {
	d, st, _, sp := newSprintDetailStore(t)

	// A valid window persists.
	d.submitDates("2026-06-20 2026-07-04")
	if d.err != "" {
		t.Fatalf("valid dates errored: %s", d.err)
	}
	reloaded, _ := st.GetSprint(sp.ID)
	if reloaded.StartAt == nil || reloaded.EndAt == nil {
		t.Fatalf("dates not saved: %+v", reloaded)
	}

	// End-before-start is rejected and leaves the stored window untouched.
	d.submitDates("2026-07-04 2026-06-20")
	if d.err == "" {
		t.Error("end-before-start should surface an error")
	}
	again, _ := st.GetSprint(sp.ID)
	if again.StartAt == nil || again.EndAt == nil || !again.EndAt.Equal(*reloaded.EndAt) {
		t.Error("a rejected edit should not change the stored window")
	}

	// A malformed date is rejected too.
	d.submitDates("not-a-date")
	if d.err == "" {
		t.Error("a malformed date should surface an error")
	}

	// Blank clears the window.
	d.submitDates("")
	cleared, _ := st.GetSprint(sp.ID)
	if cleared.StartAt != nil || cleared.EndAt != nil {
		t.Errorf("blank should clear the window, got %+v", cleared)
	}
}

func TestSprintDetailOpenIssueEmitsMsg(t *testing.T) {
	d, st, p, sp := newSprintDetailStore(t)
	iss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "open me"})
	st.AddIssueToSprint(iss.ID, &sp.ID)
	d.reload()
	d.focus = spIssues
	d.issueSel = 0

	cmd, back := d.keyIssues(keyPress("enter"))
	if back {
		t.Fatal("opening an issue should not leave the sprint view")
	}
	if cmd == nil {
		t.Fatal("expected a command to open the issue")
	}
	msg, ok := cmd().(openIssueDetailMsg)
	if !ok || msg.issueID != iss.ID {
		t.Errorf("expected openIssueDetailMsg for issue %d, got %#v", iss.ID, cmd())
	}
}

func TestSprintDetailEscReturns(t *testing.T) {
	d, _, _, _ := newSprintDetailStore(t)
	if _, back := d.updateViewing(keyPress("esc")); !back {
		t.Error("esc should return to the Sprints list")
	}
}

func TestSprintDetailFocusCycle(t *testing.T) {
	d, _, _, _ := newSprintDetailStore(t)
	if d.focus != spGoal {
		t.Fatalf("focus should start on Goal, got %v", d.focus)
	}
	want := []sprintPanel{spProgress, spIssues, spGoal}
	for i, w := range want {
		d.focusPanel(+1)
		if d.focus != w {
			t.Errorf("tab %d: focus=%v want %v", i+1, d.focus, w)
		}
	}
}

// --- root-model wiring -------------------------------------------------------

func TestSprintsHeaderEnterOpensSprintDetail(t *testing.T) {
	m, _ := newTestModel(t)
	seedSprints(t, &m)
	m.sprintSel = 0 // the first row is a sprint header

	header, ok := m.selectedSprintItem()
	if !ok || header.kind != itemHeader {
		t.Fatal("expected a sprint header under the cursor")
	}
	next, _ := m.Update(keyPress("enter"))
	m = next.(Model)
	if m.mode != modeSprintDetail || m.sprintDetail == nil {
		t.Fatalf("enter on a header should open the sprint detail, mode=%v", m.mode)
	}
	if m.sprintDetail.sprintID != header.sprint.ID {
		t.Errorf("opened sprint %d, want %d", m.sprintDetail.sprintID, header.sprint.ID)
	}

	next, _ = m.Update(keyPress("esc"))
	m = next.(Model)
	if m.mode != modeNormal || m.sprintDetail != nil {
		t.Errorf("esc should close the sprint detail, mode=%v", m.mode)
	}
}

func TestSprintDetailDrillIntoIssueAndBack(t *testing.T) {
	m, _ := newTestModel(t)
	seedSprints(t, &m)
	// The active sprint (first row) carries issues; grab one to drill into.
	iss := m.sprints.sprints[0].issues[0]

	// Open the sprint detail, then the issue from within it.
	m.sprintDetail = newSprintDetail(m.store, m.theme, m.sprints.sprints[0].sprint.ID, m.width, m.height)
	m.mode = modeSprintDetail
	next, _ := m.Update(openIssueDetailMsg{issueID: iss.ID})
	m = next.(Model)
	if m.mode != modeDetail || m.detail == nil {
		t.Fatalf("opening an issue should show the ticket detail, mode=%v", m.mode)
	}
	if m.sprintDetail == nil {
		t.Fatal("the sprint detail should be kept beneath the issue")
	}

	// Leaving the issue returns to the sprint detail, not the list.
	next, _ = m.Update(keyPress("esc"))
	m = next.(Model)
	if m.mode != modeSprintDetail || m.detail != nil {
		t.Fatalf("esc from the issue should return to the sprint detail, mode=%v", m.mode)
	}
	// Leaving the sprint detail returns to the list.
	next, _ = m.Update(keyPress("esc"))
	m = next.(Model)
	if m.mode != modeNormal || m.sprintDetail != nil {
		t.Errorf("esc from the sprint detail should return to the list, mode=%v", m.mode)
	}
}

// --- golden renders ----------------------------------------------------------

// stageSprintDetail builds a Sprint view directly over in-memory data (no
// store), with the clock pinned to 2026-06-27 UTC, so the burndown and day
// counters render deterministically regardless of the machine's timezone.
func stageSprintDetail(sprint core.Sprint, issues []core.Issue, w, h int) *sprintDetailModel {
	d := &sprintDetailModel{
		theme:    theme.New(),
		sprint:   sprint,
		issues:   issues,
		statuses: testStatuses(),
		clock:    func() time.Time { tm, _ := time.Parse("2006-01-02", "2026-06-27"); return tm },
		width:    w,
		height:   h,
	}
	d.recompute()
	return d
}

func iss(key, title string, ty core.IssueType, status int64, pts int) core.Issue {
	return core.Issue{Key: key, Title: title, Type: ty, StatusID: status, StoryPoints: ptr(pts)}
}

// activeSprintFixture is a mid-flight sprint with a goal, a dated window and a
// spread of estimated work across every column — the showcase state.
func activeSprintFixture() (core.Sprint, []core.Issue) {
	start := mustParseUTC("2026-06-20")
	end := mustParseUTC("2026-07-04")
	sp := core.Sprint{
		Name: "Sprint 7", Goal: "Ship the Sprint dashboard: goal, burndown and a way into every issue.",
		State: core.SprintActive, StartAt: &start, EndAt: &end,
	}
	mk := func(i core.Issue, day string) core.Issue { i.UpdatedAt = mustParseUTC(day); return i }
	issues := []core.Issue{
		mk(iss("JEE-1", "Design the sprint dashboard layout", core.TypeStory, 4, 5), "2026-06-22"),
		mk(iss("JEE-2", "Hand-roll the braille burndown chart", core.TypeTask, 4, 3), "2026-06-25"),
		iss("JEE-3", "Wire the Progress readout and pace", core.TypeTask, 3, 5),
		iss("JEE-4", "Status breakdown bars by category", core.TypeTask, 2, 3),
		iss("JEE-5", "Navigate into a sprint's issues", core.TypeTask, 2, 2),
		iss("JEE-6", "Edit the sprint window in-view", core.TypeBug, 1, 2),
		iss("JEE-7", "Polish the empty and future states", core.TypeTask, 1, 3),
	}
	return sp, issues
}

func TestGoldenSprintDetailActive(t *testing.T) {
	sp, issues := activeSprintFixture()
	d := stageSprintDetail(sp, issues, 100, 30)
	goldenFile(t, "sprint_detail_active", stripANSI(d.View()))
}

func TestGoldenSprintDetailNarrow(t *testing.T) {
	sp, issues := activeSprintFixture()
	d := stageSprintDetail(sp, issues, 70, 30)
	goldenFile(t, "sprint_detail_narrow", stripANSI(d.View()))
}

func TestGoldenSprintDetailFuture(t *testing.T) {
	start := mustParseUTC("2026-07-10")
	end := mustParseUTC("2026-07-24")
	sp := core.Sprint{
		Name: "Sprint 8", Goal: "Harden the MCP surface and ship the run timeline.",
		State: core.SprintFuture, StartAt: &start, EndAt: &end,
	}
	issues := []core.Issue{
		iss("JEE-8", "Audit every MCP tool's output", core.TypeTask, 1, 5),
		iss("JEE-9", "Run timeline view", core.TypeStory, 1, 8),
		iss("JEE-10", "Schedule digest", core.TypeTask, 1, 3),
	}
	d := stageSprintDetail(sp, issues, 100, 30)
	goldenFile(t, "sprint_detail_future", stripANSI(d.View()))
}

func TestGoldenSprintDetailCompleted(t *testing.T) {
	start := mustParseUTC("2026-06-01")
	end := mustParseUTC("2026-06-14")
	sp := core.Sprint{
		Name: "Sprint 6", Goal: "Make the board a SCRUM board scoped to the active sprint.",
		State: core.SprintCompleted, StartAt: &start, EndAt: &end,
	}
	mk := func(i core.Issue, day string) core.Issue { i.UpdatedAt = mustParseUTC(day); return i }
	issues := []core.Issue{
		mk(iss("JEE-3", "Scope the board to the active sprint", core.TypeStory, 4, 5), "2026-06-05"),
		mk(iss("JEE-4", "Roll incomplete issues back on finish", core.TypeTask, 4, 3), "2026-06-09"),
		mk(iss("JEE-5", "Scroll overflowing board columns", core.TypeTask, 4, 5), "2026-06-12"),
	}
	d := stageSprintDetail(sp, issues, 100, 30)
	goldenFile(t, "sprint_detail_completed", stripANSI(d.View()))
}

func TestGoldenSprintDetailIssueBasis(t *testing.T) {
	start := mustParseUTC("2026-06-20")
	end := mustParseUTC("2026-07-04")
	sp := core.Sprint{
		Name: "Triage", Goal: "Clear the inbound bug queue before the next release.",
		State: core.SprintActive, StartAt: &start, EndAt: &end,
	}
	// No story points anywhere → the view falls back to an issue-count basis.
	noPts := func(key, title string, ty core.IssueType, status int64, day string) core.Issue {
		i := core.Issue{Key: key, Title: title, Type: ty, StatusID: status}
		if day != "" {
			i.UpdatedAt = mustParseUTC(day)
		}
		return i
	}
	issues := []core.Issue{
		noPts("BUG-1", "Crash on empty sprint window", core.TypeBug, 4, "2026-06-23"),
		noPts("BUG-2", "Wrong day counter across DST", core.TypeBug, 4, "2026-06-26"),
		noPts("BUG-3", "Burndown overflows on narrow widths", core.TypeBug, 2, ""),
		noPts("BUG-4", "Goal wraps mid-word", core.TypeBug, 1, ""),
		noPts("BUG-5", "Pace reads behind when on track", core.TypeBug, 1, ""),
	}
	d := stageSprintDetail(sp, issues, 100, 30)
	goldenFile(t, "sprint_detail_issue_basis", stripANSI(d.View()))
}

func TestGoldenSprintDetailEmpty(t *testing.T) {
	start := mustParseUTC("2026-06-20")
	end := mustParseUTC("2026-07-04")
	sp := core.Sprint{Name: "Sprint 9", State: core.SprintActive, StartAt: &start, EndAt: &end}
	d := stageSprintDetail(sp, nil, 100, 30)
	goldenFile(t, "sprint_detail_empty", stripANSI(d.View()))
}

func mustParseUTC(s string) time.Time {
	tm, _ := time.Parse("2006-01-02", s)
	return tm
}
