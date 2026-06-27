package tui

import (
	"fmt"
	"sort"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/store"
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

// flatIssues is the sprints' issues in display order.
func (sd sprintsData) flatIssues() []core.Issue {
	var out []core.Issue
	for _, sr := range sd.sprints {
		out = append(out, sr.issues...)
	}
	return out
}

// sprintItemKind distinguishes the two things the Sprints cursor can land on.
type sprintItemKind int

const (
	itemHeader sprintItemKind = iota // a sprint header — start/finish/delete/add-issue
	itemIssue                        // an issue inside a sprint — open/move/return-to-backlog
)

// sprintItem is one selectable row in the Sprints view. Both kinds carry the
// containing sprint, so an action on an issue knows which sprint it belongs to.
type sprintItem struct {
	kind   sprintItemKind
	sprint core.Sprint
	issue  core.Issue
}

// items is the flat list the Sprints cursor (m.sprintSel) walks: each sprint's
// header followed by its issues. Empty sprints contribute just their header, so
// they can still be started, deleted, or have an issue added.
func (sd sprintsData) items() []sprintItem {
	var out []sprintItem
	for _, sr := range sd.sprints {
		out = append(out, sprintItem{kind: itemHeader, sprint: sr.sprint})
		for _, iss := range sr.issues {
			out = append(out, sprintItem{kind: itemIssue, sprint: sr.sprint, issue: iss})
		}
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
	n := len(m.sprints.items())
	if m.sprintSel >= n {
		m.sprintSel = n - 1
	}
	if m.sprintSel < 0 {
		m.sprintSel = 0
	}
}

// selectedSprintItem returns the row (sprint header or issue) the cursor is on.
func (m Model) selectedSprintItem() (sprintItem, bool) {
	items := m.sprints.items()
	if m.sprintSel < 0 || m.sprintSel >= len(items) {
		return sprintItem{}, false
	}
	return items[m.sprintSel], true
}

// selectedSprintIssue returns the issue the cursor is on, if it's on an issue
// (not a sprint header).
func (m Model) selectedSprintIssue() (core.Issue, bool) {
	if it, ok := m.selectedSprintItem(); ok && it.kind == itemIssue {
		return it.issue, true
	}
	return core.Issue{}, false
}

// updateSprints handles keys specific to the Sprints view. The cursor walks both
// sprint headers and the issues within them; the action a key takes depends on
// which kind is selected, so one key set covers the whole planning surface.
func (m Model) updateSprints(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		m.sprintSel--
		m.clampSprintSel()
	case key.Matches(msg, m.keys.Down):
		m.sprintSel++
		m.clampSprintSel()
	case key.Matches(msg, m.keys.New): // n: plan a new sprint, regardless of selection
		if m.active.ID != 0 {
			m.form = newCreateSprintForm()
			m.mode = modeForm
			return m, m.form.focusCmd()
		}
	case key.Matches(msg, m.keys.Enter):
		if it, ok := m.selectedSprintItem(); ok && it.kind == itemIssue {
			m.detail = newDetail(m.store, m.runMgr, m.sched, m.theme, it.issue.ID, m.width, m.height)
			m.mode = modeDetail
		}
	case key.Matches(msg, m.keys.Cycle): // s: advance the selected sprint's state
		if it, ok := m.selectedSprintItem(); ok && it.kind == itemHeader {
			return m.cycleSprintState(it.sprint)
		}
	case key.Matches(msg, m.keys.Assign): // a: header → add an issue; issue → move it
		if it, ok := m.selectedSprintItem(); ok {
			if it.kind == itemHeader {
				return m.openBacklogPicker(it.sprint.ID, it.sprint.Name)
			}
			return m.openSprintPicker(it.issue)
		}
	case key.Matches(msg, m.keys.Unsprint): // ⌫: pull an issue back to the backlog
		if it, ok := m.selectedSprintItem(); ok && it.kind == itemIssue {
			if err := m.store.AddIssueToSprint(it.issue.ID, nil); err != nil {
				return m, reportErr(err)
			}
			m.reload()
			return m, toast(it.issue.Key + " → backlog")
		}
	case key.Matches(msg, m.keys.Delete): // x: delete the selected sprint
		if it, ok := m.selectedSprintItem(); ok && it.kind == itemHeader {
			return m.confirmDeleteSprint(it.sprint)
		}
	}
	return m, nil
}

// cycleSprintState advances a sprint through its lifecycle: future → active →
// completed → future, persisting the change so the board and agents see it.
// Starting and finishing go through StartSprint/CompleteSprint so the SCRUM
// rules hold — one active sprint per project, and a finished sprint's unfinished
// issues roll back to the backlog.
func (m Model) cycleSprintState(sp core.Sprint) (tea.Model, tea.Cmd) {
	var (
		err  error
		verb string
	)
	switch sp.State {
	case core.SprintFuture:
		err, verb = m.store.StartSprint(sp.ID), "started"
	case core.SprintActive:
		err, verb = m.store.CompleteSprint(sp.ID), "completed"
	default: // completed → reopen for re-planning
		sp.State, verb = core.SprintFuture, "reopened"
		err = m.store.UpdateSprint(sp)
	}
	if err != nil {
		return m, reportErr(err)
	}
	m.reload()
	return m, toast(verb + " " + sp.Name)
}

// sprintCycleVerb is the footer label for the s key on a sprint header, matching
// what the next press will actually do.
func sprintCycleVerb(s core.SprintState) string {
	switch s {
	case core.SprintFuture:
		return "start"
	case core.SprintActive:
		return "finish"
	default:
		return "reopen"
	}
}

// confirmDeleteSprint asks before removing a sprint; its issues fall back to the
// backlog (the schema's ON DELETE SET NULL), they are not destroyed.
func (m Model) confirmDeleteSprint(sp core.Sprint) (tea.Model, tea.Cmd) {
	m.confirm = fmt.Sprintf("Delete sprint %q? Its issues return to the backlog.", sp.Name)
	id := sp.ID
	st := m.store
	m.onConfirm = func() tea.Cmd {
		if err := st.DeleteSprint(id); err != nil {
			return reportErr(err)
		}
		return toast("deleted sprint")
	}
	m.mode = modeConfirm
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
	selLine, itemIdx := 0, 0
	for si, sr := range m.sprints.sprints {
		if si > 0 {
			lines = append(lines, "")
		}
		if itemIdx == m.sprintSel {
			selLine = len(lines)
		}
		lines = append(lines, m.renderSprintHeader(sr, itemIdx == m.sprintSel))
		itemIdx++
		if len(sr.issues) == 0 {
			lines = append(lines, "      "+t.HelpDesc.Render("— no issues —"))
			continue
		}
		for _, iss := range sr.issues {
			if itemIdx == m.sprintSel {
				selLine = len(lines)
			}
			lines = append(lines, m.renderIssueRow(iss, itemIdx == m.sprintSel, m.sprints.statuses, 4))
			itemIdx++
		}
	}

	start, end := scrollWindow(selLine, len(lines), height)
	return fitHeight(lipgloss.JoinVertical(lipgloss.Left, lines[start:end]...), height)
}

func (m Model) renderSprintHeader(sr sprintRow, selected bool) string {
	t := m.theme
	c := t.SprintStateColor(sr.sprint.State)
	// Cap the name so a long one never pushes the state/dates/meta off a narrow
	// terminal — the rest of the row is short and fixed.
	nameW := m.width - 48
	if nameW < 12 {
		nameW = 12
	}
	left := []cell{
		cText("  "),
		cFg("● ", c),
		cKey(truncate(sr.sprint.Name, nameW), t.P.TextPrimary),
		cText("  "),
		cFg(string(sr.sprint.State), c),
	}
	if dr := sprintDates(sr.sprint); dr != "" {
		left = append(left, cText("  "), cFg(dr, t.P.TextSubtle))
	}
	meta := fmt.Sprintf("%d issue%s", len(sr.issues), pluralS(len(sr.issues)))
	if sr.points > 0 {
		meta += fmt.Sprintf(" · %d pts", sr.points)
	}
	right := []cell{cFg(meta, t.P.TextSubtle), cText("  ")}
	return listRow(t, m.width, selected, left, right)
}

func (m Model) sprintsEmpty() string {
	t := m.theme
	return lipgloss.JoinVertical(lipgloss.Center,
		t.Title.Render("No sprints yet"),
		"",
		t.HelpDesc.Render("Sprints group issues into time-boxes."),
		t.HelpDesc.Render("Press n to plan your first sprint."),
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
