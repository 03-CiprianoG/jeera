package tui

import (
	"testing"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// seedBoard creates a project with a spread of issues exercising priority,
// assignee, points and tags so the card rendering is fully covered.
func seedBoard(t *testing.T, m *Model) {
	t.Helper()
	st := m.store
	p := seedProject(t, st)
	statuses, _ := st.ListStatuses(p.ID)

	pts := 5
	a, _ := st.CreateIssue(core.Issue{
		ProjectID: p.ID, Title: "Design the kanban board layout", Type: core.TypeStory,
		Priority: core.PriorityHigh, StoryPoints: &pts,
		Assignee: core.Assignee{Provider: core.ProviderClaude, Model: "opus", Effort: core.EffortHigh},
	})
	tag, _ := st.CreateTag(core.Tag{ProjectID: p.ID, Name: "ui"})
	st.TagIssue(a.ID, tag.ID)

	st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "Wire up the MCP status pill", Type: core.TypeTask, Priority: core.PriorityMedium})

	b, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "Fix card overflow on narrow widths", Type: core.TypeBug, Priority: core.PriorityHighest})
	st.TransitionIssue(b.ID, statuses[1].ID) // In Progress

	c, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "Set up the project scaffold", Type: core.TypeTask, Priority: core.PriorityLow})
	st.TransitionIssue(c.ID, statuses[3].ID) // Done

	d, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "Review the theme tokens", Type: core.TypeTask, Priority: core.PriorityMedium})
	st.TransitionIssue(d.ID, statuses[2].ID) // In Review

	m.reload()
}

func TestGoldenWelcome(t *testing.T) {
	m, _ := newTestModel(t) // no projects
	goldenFile(t, "welcome", render(m))
}

func TestGoldenBoard(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m)
	m.colIdx, m.cardIdx = 0, 0
	goldenFile(t, "board", render(m))
}

func TestGoldenHelp(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m)
	m.mode = modeHelp
	goldenFile(t, "help", render(m))
}

func TestGoldenForm(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m)
	m.form = newCreateIssueForm(0)
	m.mode = modeForm
	goldenFile(t, "form", render(m))
}

func TestGoldenMCPOff(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m)
	m.mode = modeMCP
	goldenFile(t, "mcp_off", render(m))
}

func TestGoldenProjects(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m)
	m.mode = modeProjects
	goldenFile(t, "projects", render(m))
}
