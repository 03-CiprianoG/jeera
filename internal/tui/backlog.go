package tui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/store"
	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

// backlogData is everything the Backlog view needs: the project's unsprinted
// issues and its statuses, so each row can show its category at a glance.
type backlogData struct {
	issues      []core.Issue
	statuses    map[int64]core.Status
	sprintCount int // sprints in the project, so the view offers "assign" only when one exists
}

// issueFilterUnsprinted selects a project's issues that aren't in any sprint —
// the backlog. Shared by the Backlog view and the "add issue to sprint" picker.
func issueFilterUnsprinted(projectID int64) store.IssueFilter {
	return store.IssueFilter{ProjectID: projectID, Unsprinted: true}
}

// loadBacklog reads a project's unsprinted issues for the Backlog view. It mirrors
// loadBoard/loadSprints, propagating store errors.
func loadBacklog(st *store.Store, projectID int64) (backlogData, error) {
	if st == nil || projectID == 0 {
		return backlogData{}, nil
	}
	issues, err := st.ListIssues(issueFilterUnsprinted(projectID))
	if err != nil {
		return backlogData{}, err
	}
	statusList, err := st.ListStatuses(projectID)
	if err != nil {
		return backlogData{}, err
	}
	statuses := make(map[int64]core.Status, len(statusList))
	for _, s := range statusList {
		statuses[s.ID] = s
	}
	sprints, err := st.ListSprints(projectID)
	if err != nil {
		return backlogData{}, err
	}
	return backlogData{issues: issues, statuses: statuses, sprintCount: len(sprints)}, nil
}

func (m *Model) clampBacklogSel() {
	n := len(m.backlog.issues)
	if m.backlogSel >= n {
		m.backlogSel = n - 1
	}
	if m.backlogSel < 0 {
		m.backlogSel = 0
	}
}

func (m Model) selectedBacklogIssue() (core.Issue, bool) {
	if m.backlogSel < 0 || m.backlogSel >= len(m.backlog.issues) {
		return core.Issue{}, false
	}
	return m.backlog.issues[m.backlogSel], true
}

// updateBacklog handles keys specific to the Backlog view: navigate the
// unsprinted issues, open one, create a new one, or assign one to a sprint.
func (m Model) updateBacklog(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		m.backlogSel--
		m.clampBacklogSel()
	case key.Matches(msg, m.keys.Down):
		m.backlogSel++
		m.clampBacklogSel()
	case key.Matches(msg, m.keys.New):
		if m.active.ID != 0 {
			m.form = newCreateIssueForm()
			m.mode = modeForm
			return m, m.form.focusCmd()
		}
	case key.Matches(msg, m.keys.Enter):
		if iss, ok := m.selectedBacklogIssue(); ok {
			m.detail = newDetail(m.store, m.runMgr, m.sched, m.theme, iss.ID, m.width, m.height)
			m.mode = modeDetail
		}
	case key.Matches(msg, m.keys.Assign):
		if iss, ok := m.selectedBacklogIssue(); ok {
			if m.backlog.sprintCount == 0 {
				return m, toast("no sprints yet — create one in Sprints")
			}
			return m.openSprintPicker(iss)
		}
	}
	return m, nil
}

// renderBacklog draws the unsprinted issues in a region exactly height rows tall,
// scrolling so the selection stays on screen.
func (m Model) renderBacklog(height int) string {
	t := m.theme
	if m.active.ID == 0 {
		return m.center(t.Empty.Render("Create a project to triage a backlog."), height)
	}
	if len(m.backlog.issues) == 0 {
		return m.center(m.backlogEmpty(), height)
	}

	header := t.Label.Render("Backlog") + "   " + t.HelpDesc.Render(fmt.Sprintf("%d unsprinted", len(m.backlog.issues)))
	lines := []string{header, ""}
	start, end := scrollWindow(m.backlogSel, len(m.backlog.issues), height-2)
	for i := start; i < end; i++ {
		lines = append(lines, m.renderIssueRow(m.backlog.issues[i], i == m.backlogSel, m.backlog.statuses))
	}
	return fitHeight(lipgloss.JoinVertical(lipgloss.Left, lines...), height)
}

// renderIssueRow renders one issue as a single line — status dot, priority glyph,
// key and title, with story points right-aligned. Shared by the Backlog and
// Sprints views so an issue reads identically wherever it appears.
func (m Model) renderIssueRow(iss core.Issue, selected bool, statuses map[int64]core.Status) string {
	t := m.theme
	cat := core.CategoryTodo
	if s, ok := statuses[iss.StatusID]; ok {
		cat = s.Category
	}
	stDot := lipgloss.NewStyle().Foreground(t.CategoryColor(cat)).Render("●")
	pri := lipgloss.NewStyle().Foreground(t.PriorityColor(iss.Priority)).Render(theme.PriorityGlyph(iss.Priority))

	marker := "  "
	titleStyle := t.CardTitle
	if selected {
		marker = t.HelpKey.Render("▸ ")
		titleStyle = titleStyle.Bold(true)
	}
	titleW := m.width - 24
	if titleW < 8 {
		titleW = 8
	}
	left := marker + stDot + " " + pri + " " +
		t.CardKey.Render(fmt.Sprintf("%-10s", iss.Key)) + " " +
		titleStyle.Render(truncate(iss.Title, titleW))
	if iss.StoryPoints != nil {
		return spread(left, t.CardMeta.Render(fmt.Sprintf("%dpt", *iss.StoryPoints)), m.width-2)
	}
	return left
}

func (m Model) backlogEmpty() string {
	t := m.theme
	return lipgloss.JoinVertical(lipgloss.Center,
		t.Title.Render("Backlog is clear"),
		"",
		t.HelpDesc.Render("Every issue is in a sprint, or there are none yet."),
		t.HelpDesc.Render("Press n to add one."),
	)
}
