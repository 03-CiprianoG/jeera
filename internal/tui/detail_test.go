package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/run"
	"github.com/03-CiprianoG/jeera/internal/schedule"
	"github.com/03-CiprianoG/jeera/internal/store"
	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

func newDetailForTest(t *testing.T) (*detailModel, *store.Store, core.Issue) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/jeera.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, _ := st.CreateProject(core.Project{Name: "Jeera", KeyPrefix: "JEE", RepoPath: "/tmp/jeera"})
	iss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "Build the board", Type: core.TypeStory})
	d := newDetail(st, nil, nil /* no run manager or scheduler in unit tests */, theme.New(), iss.ID, 100, 30)
	return d, st, iss
}

func TestDetailCyclePriority(t *testing.T) {
	d, st, iss := newDetailForTest(t)
	d.field = dfPriority
	before := d.issue.Priority
	d.cycleField(+1)
	if d.issue.Priority == before {
		t.Errorf("priority did not change")
	}
	reloaded, _ := st.GetIssue(iss.ID)
	if reloaded.Priority != d.issue.Priority {
		t.Errorf("priority not persisted: store=%q view=%q", reloaded.Priority, d.issue.Priority)
	}
}

func TestDetailCycleStatusTransitions(t *testing.T) {
	d, st, iss := newDetailForTest(t)
	d.field = dfStatus
	d.cycleField(+1) // To Do -> In Progress
	reloaded, _ := st.GetIssue(iss.ID)
	if reloaded.StatusID != d.statuses[1].ID {
		t.Errorf("status not transitioned to second column")
	}
}

func TestDetailAssigneeCycle(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	d.field = dfProvider
	d.cycleField(+1) // sets a default assignee
	if d.issue.Assignee.IsZero() {
		t.Fatalf("cycling provider should set an assignee")
	}
	d.field = dfModel
	before := d.issue.Assignee.Model
	d.cycleField(+1)
	if d.issue.Assignee.Model == before {
		t.Errorf("model did not cycle")
	}
	d.field = dfEffort
	beforeE := d.issue.Assignee.Effort
	d.cycleField(+1)
	if d.issue.Assignee.Effort == beforeE {
		t.Errorf("effort did not cycle")
	}
}

func TestDetailEditPoints(t *testing.T) {
	d, st, iss := newDetailForTest(t)
	d.startInput(ikPoints, "")
	d.input.SetValue("8")
	d.submitInput("8")
	reloaded, _ := st.GetIssue(iss.ID)
	if reloaded.StoryPoints == nil || *reloaded.StoryPoints != 8 {
		t.Errorf("points not saved: %v", reloaded.StoryPoints)
	}
}

func TestDetailAddTagAndComment(t *testing.T) {
	d, st, iss := newDetailForTest(t)
	d.inputKind = ikTag
	d.submitInput("backend")
	tags, _ := st.ListIssueTags(iss.ID)
	if len(tags) != 1 || tags[0].Name != "backend" {
		t.Errorf("tag not added: %+v", tags)
	}

	d.inputKind = ikComment
	d.submitInput("looks good")
	comments, _ := st.ListComments(iss.ID)
	if len(comments) != 1 || comments[0].Body != "looks good" {
		t.Errorf("comment not added: %+v", comments)
	}
}

func TestDetailEditDescriptionSaves(t *testing.T) {
	d, st, iss := newDetailForTest(t)
	d.startEditDesc()
	d.desc.SetValue("## Goal\n\nShip it.")
	if _, back := d.updateEditDesc(keyPress2("ctrl+s")); back {
		t.Error("ctrl+s should not return to board")
	}
	reloaded, _ := st.GetIssue(iss.ID)
	if reloaded.Description != "## Goal\n\nShip it." {
		t.Errorf("description not saved: %q", reloaded.Description)
	}
}

func TestDetailCycleSprint(t *testing.T) {
	d, st, iss := newDetailForTest(t)
	s1, _ := st.CreateSprint(core.Sprint{ProjectID: iss.ProjectID, Name: "S1"})
	d.reload()
	d.field = dfSprint

	d.cycleField(+1) // none -> S1
	if d.issue.SprintID == nil || *d.issue.SprintID != s1.ID {
		t.Fatalf("expected sprint S1, got %v", d.issue.SprintID)
	}
	d.cycleField(+1) // S1 -> back to none (opts = [nil, S1])
	if d.issue.SprintID != nil {
		t.Errorf("expected back to none, got %v", *d.issue.SprintID)
	}
}

func TestDetailModelCycleFromOutOfCatalog(t *testing.T) {
	d, st, iss := newDetailForTest(t)
	// An out-of-catalog assignee (codex provider, claude model) can be persisted
	// via the MCP set_assignee tool. Cycling Model must land on the provider's
	// first catalog model, not skip it.
	iss.Assignee = core.Assignee{Provider: core.ProviderCodex, Model: "opus", Effort: core.EffortMedium}
	if err := st.UpdateIssue(iss); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	d.reload()
	d.field = dfModel
	d.cycleField(+1)

	want := core.ProviderModels(core.ProviderCodex)[0]
	if d.issue.Assignee.Model != want {
		t.Errorf("cycle from out-of-catalog model = %q, want first catalog model %q", d.issue.Assignee.Model, want)
	}
}

func TestDetailCycleEmptyListsNoPanic(t *testing.T) {
	d, _, _ := newDetailForTest(t) // no sprints, no epics
	d.field = dfSprint
	d.cycleField(+1) // opts = [nil] only
	if d.issue.SprintID != nil {
		t.Errorf("with no sprints, sprint should stay none")
	}
	d.field = dfEpic
	d.cycleField(+1)
	if d.issue.EpicID != nil {
		t.Errorf("with no epics, epic should stay none")
	}
}

func TestDetailEscReturnsToBoard(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	_, back := d.updateViewing(keyPress2("esc"))
	if !back {
		t.Error("esc should return to the board")
	}
}

// detailWithScheduler builds a detail model backed by a live scheduler, for the
// "Schedule Start" flow.
func detailWithScheduler(t *testing.T) (*detailModel, *store.Store, core.Issue) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/jeera.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, _ := st.CreateProject(core.Project{Name: "Jeera", KeyPrefix: "JEE", RepoPath: "/tmp/jeera"})
	iss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "nightly job"})

	mgr := run.NewManager(st, t.TempDir(), func() string { return "" }, nil)
	sched, err := schedule.New(st, mgr)
	if err != nil {
		t.Fatalf("schedule.New: %v", err)
	}
	t.Cleanup(func() { _ = sched.Shutdown() })
	if err := sched.Start(); err != nil {
		t.Fatalf("scheduler start: %v", err)
	}
	d := newDetail(st, mgr, sched, theme.New(), iss.ID, 100, 30)
	return d, st, iss
}

func TestDetailScheduleAndUnschedule(t *testing.T) {
	d, st, iss := detailWithScheduler(t)

	d.inputKind = ikCron
	d.submitInput("0 9 * * *")
	d.reload()
	if d.err != "" {
		t.Fatalf("unexpected error scheduling: %s", d.err)
	}
	if len(d.schedules) != 1 || d.schedules[0].CronSpec != "0 9 * * *" {
		t.Fatalf("expected one schedule, got %+v", d.schedules)
	}
	// Persisted to the store, with a job binding and a future next-run.
	stored, _ := st.ListSchedules(iss.ID)
	if len(stored) != 1 || stored[0].JobUUID == "" || stored[0].NextRun == nil {
		t.Errorf("schedule not persisted with a live job: %+v", stored)
	}

	d.unschedule()
	if len(d.schedules) != 0 {
		t.Errorf("schedule should be removed, got %+v", d.schedules)
	}
}

func TestDetailScheduleRejectsBadCron(t *testing.T) {
	d, _, _ := detailWithScheduler(t)
	d.inputKind = ikCron
	d.submitInput("not a cron")
	if d.err == "" {
		t.Error("a bad cron spec should surface an error")
	}
	d.reload()
	if len(d.schedules) != 0 {
		t.Errorf("a rejected schedule should not persist, got %+v", d.schedules)
	}
}

func TestGoldenDetail(t *testing.T) {
	d, st, iss := newDetailForTest(t)
	// Enrich the issue so the view exercises description, assignee, points, tags.
	iss.Description = "## Goal\n\nBuild a **calm**, keyboard-driven board.\n\n- columns by status\n- model-assignee cards"
	pts := 5
	iss.StoryPoints = &pts
	iss.Priority = core.PriorityHigh
	iss.Assignee = core.Assignee{Provider: core.ProviderClaude, Model: "opus", Effort: core.EffortHigh}
	st.UpdateIssue(iss)
	tag, _ := st.CreateTag(core.Tag{ProjectID: iss.ProjectID, Name: "ui"})
	st.TagIssue(iss.ID, tag.ID)
	st.AddComment(core.Comment{IssueID: iss.ID, Body: "Kicked off."})
	d.reload()

	goldenFile(t, "detail", stripANSI(d.View()))
}

// keyPress2 is keyPress but also handles ctrl+s for the detail tests.
func keyPress2(s string) tea.KeyPressMsg {
	if s == "ctrl+s" {
		return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	}
	return keyPress(s)
}
