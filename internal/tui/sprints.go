package tui

import (
	"fmt"
	"sort"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/store"
	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

// sprintsData is everything the Sprints view needs for the active project: the
// sprints with their issues and points, plus the project's statuses so each
// issue can show its category at a glance.
type sprintsData struct {
	sprints  []sprintRow
	statuses map[int64]core.Status
}

type sprintRow struct {
	sprint core.Sprint
	issues []core.Issue
	points int // Σ story points across the sprint's issues
}

// flatIssues is the sprints' issues in display order, which is also the order
// the single Sprints cursor (m.sprintSel) walks.
func (sd sprintsData) flatIssues() []core.Issue {
	var out []core.Issue
	for _, sr := range sd.sprints {
		out = append(out, sr.issues...)
	}
	return out
}

// loadSprints reads a project's sprints and the issues in each, grouped for the
// Sprints view. It mirrors loadBoard in data.go, propagating store errors so the
// caller can surface them rather than silently showing an empty view.
func loadSprints(st *store.Store, projectID int64) (sprintsData, error) {
	if st == nil || projectID == 0 {
		return sprintsData{}, nil
	}
	sprints, err := st.ListSprints(projectID)
	if err != nil {
		return sprintsData{}, err
	}
	statusList, err := st.ListStatuses(projectID)
	if err != nil {
		return sprintsData{}, err
	}
	statuses := make(map[int64]core.Status, len(statusList))
	for _, s := range statusList {
		statuses[s.ID] = s
	}
	rows := make([]sprintRow, 0, len(sprints))
	for _, sp := range sprints {
		id := sp.ID
		issues, err := st.ListIssues(store.IssueFilter{ProjectID: projectID, SprintID: &id})
		if err != nil {
			return sprintsData{}, err
		}
		pts := 0
		for _, iss := range issues {
			if iss.StoryPoints != nil {
				pts += *iss.StoryPoints
			}
		}
		rows = append(rows, sprintRow{sprint: sp, issues: issues, points: pts})
	}
	// Lead with the sprint that's in flight, then what's coming, then what's done —
	// the order a planner reads top to bottom. ListSprints' newest-first order is
	// preserved within each group.
	sort.SliceStable(rows, func(i, j int) bool {
		return sprintStateOrder(rows[i].sprint.State) < sprintStateOrder(rows[j].sprint.State)
	})
	return sprintsData{sprints: rows, statuses: statuses}, nil
}

// sprintStateOrder ranks lifecycle states for the Sprints view: active first.
func sprintStateOrder(s core.SprintState) int {
	switch s {
	case core.SprintActive:
		return 0
	case core.SprintFuture:
		return 1
	default: // completed
		return 2
	}
}

func (m *Model) clampSprintSel() {
	n := len(m.sprints.flatIssues())
	if m.sprintSel >= n {
		m.sprintSel = n - 1
	}
	if m.sprintSel < 0 {
		m.sprintSel = 0
	}
}

// selectedSprintIssue returns the issue the Sprints cursor is on, if any.
func (m Model) selectedSprintIssue() (core.Issue, bool) {
	flat := m.sprints.flatIssues()
	if m.sprintSel < 0 || m.sprintSel >= len(flat) {
		return core.Issue{}, false
	}
	return flat[m.sprintSel], true
}

// updateSprints handles keys specific to the Sprints view: the cursor walks the
// issues across all sprints, and enter opens the selected one. Sprint headers
// are skipped — they orient, they aren't selectable.
func (m Model) updateSprints(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		m.sprintSel--
		m.clampSprintSel()
	case key.Matches(msg, m.keys.Down):
		m.sprintSel++
		m.clampSprintSel()
	case key.Matches(msg, m.keys.Enter):
		if iss, ok := m.selectedSprintIssue(); ok {
			m.detail = newDetail(m.store, m.runMgr, m.sched, m.theme, iss.ID, m.width, m.height)
			m.mode = modeDetail
		}
	}
	return m, nil
}

// renderSprints draws the sprints and their issues in a region exactly height
// rows tall, scrolling so the selected issue is always on screen.
func (m Model) renderSprints(height int) string {
	t := m.theme
	if m.active.ID == 0 {
		return m.center(t.Empty.Render("Create a project to plan sprints."), height)
	}
	if len(m.sprints.sprints) == 0 {
		return m.center(m.sprintsEmpty(), height)
	}

	var lines []string
	selLine, flatIdx := 0, 0
	for si, sr := range m.sprints.sprints {
		if si > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, m.renderSprintHeader(sr))
		if len(sr.issues) == 0 {
			lines = append(lines, "   "+t.HelpDesc.Render("— no issues —"))
			continue
		}
		for _, iss := range sr.issues {
			selected := flatIdx == m.sprintSel
			if selected {
				selLine = len(lines)
			}
			lines = append(lines, m.renderSprintIssue(iss, selected))
			flatIdx++
		}
	}

	start, end := scrollWindow(selLine, len(lines), height)
	return fitHeight(lipgloss.JoinVertical(lipgloss.Left, lines[start:end]...), height)
}

func (m Model) renderSprintHeader(sr sprintRow) string {
	t := m.theme
	c := t.SprintStateColor(sr.sprint.State)
	dot := lipgloss.NewStyle().Foreground(c).Render("●")
	// Cap the name so a long one never pushes the state/dates/meta off a narrow
	// terminal — the rest of the row is short and fixed.
	nameW := m.width - 48
	if nameW < 12 {
		nameW = 12
	}
	name := t.Title.Render(truncate(sr.sprint.Name, nameW))
	state := lipgloss.NewStyle().Foreground(c).Render(string(sr.sprint.State))

	left := dot + " " + name + "  " + state
	if dr := sprintDates(sr.sprint); dr != "" {
		left += "  " + t.HelpDesc.Render(dr)
	}

	meta := fmt.Sprintf("%d issue%s", len(sr.issues), pluralS(len(sr.issues)))
	if sr.points > 0 {
		meta += fmt.Sprintf(" · %d pts", sr.points)
	}
	return spread(left, t.CardMeta.Render(meta), m.width-2)
}

func (m Model) renderSprintIssue(iss core.Issue, selected bool) string {
	t := m.theme
	cat := core.CategoryTodo
	if s, ok := m.sprints.statuses[iss.StatusID]; ok {
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

	// Title takes whatever remains after the fixed-width lead (marker, dots, key)
	// and a right-aligned points column.
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

func (m Model) sprintsEmpty() string {
	t := m.theme
	return lipgloss.JoinVertical(lipgloss.Center,
		t.Title.Render("No sprints yet"),
		"",
		t.HelpDesc.Render("Sprints group issues into time-boxes."),
		t.HelpDesc.Render("Create one over MCP and it shows up here."),
	)
}

// sprintDates formats a sprint's window for the header, tolerating either bound
// being unset.
func sprintDates(sp core.Sprint) string {
	const f = "Jan 2"
	switch {
	case sp.StartAt != nil && sp.EndAt != nil:
		return sp.StartAt.Local().Format(f) + " – " + sp.EndAt.Local().Format(f)
	case sp.StartAt != nil:
		return "from " + sp.StartAt.Local().Format(f)
	case sp.EndAt != nil:
		return "until " + sp.EndAt.Local().Format(f)
	}
	return ""
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
