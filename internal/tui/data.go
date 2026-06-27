package tui

import (
	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/store"
)

// column is one board lane: a status and the issues currently in it.
type column struct {
	status core.Status
	cards  []core.Issue
}

// boardData is everything the board view needs for the active project. As a
// SCRUM board it is scoped to the project's active sprint: sprint is that sprint
// (nil when none is running, in which case columns carry no cards and the view
// shows a "start a sprint" state).
type boardData struct {
	sprint  *core.Sprint // the active sprint the board is scoped to; nil when none is running
	columns []column
	tags    map[int64][]string // issue id → tag names, for card rendering
}

// loadBoard reads a project's columns and the active sprint's issues from the
// store and groups them for rendering. The board is a SCRUM board, so only the
// active sprint's issues appear; with no active sprint there is nothing to show
// (the caller renders an empty state), so issue loading is skipped entirely.
func loadBoard(st *store.Store, projectID int64) (boardData, error) {
	statuses, err := st.ListStatuses(projectID)
	if err != nil {
		return boardData{}, err
	}
	active, ok, err := st.ActiveSprint(projectID)
	if err != nil {
		return boardData{}, err
	}
	var (
		issues []core.Issue
		sprint *core.Sprint
	)
	if ok {
		sprint = &active
		issues, err = st.ListIssues(store.IssueFilter{ProjectID: projectID, SprintID: &active.ID})
		if err != nil {
			return boardData{}, err
		}
	}

	byStatus := make(map[int64][]core.Issue, len(statuses))
	for _, iss := range issues {
		byStatus[iss.StatusID] = append(byStatus[iss.StatusID], iss)
	}
	cols := make([]column, len(statuses))
	for i, s := range statuses {
		cols[i] = column{status: s, cards: byStatus[s.ID]}
	}

	tags := make(map[int64][]string)
	for _, iss := range issues {
		ts, err := st.ListIssueTags(iss.ID)
		if err != nil {
			return boardData{}, err
		}
		if len(ts) > 0 {
			names := make([]string, len(ts))
			for j, t := range ts {
				names[j] = t.Name
			}
			tags[iss.ID] = names
		}
	}
	return boardData{sprint: sprint, columns: cols, tags: tags}, nil
}
