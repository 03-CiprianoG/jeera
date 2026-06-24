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

// boardData is everything the board view needs for the active project.
type boardData struct {
	columns []column
	tags    map[int64][]string // issue id → tag names, for card rendering
}

// loadBoard reads a project's columns and issues from the store and groups them
// for rendering.
func loadBoard(st *store.Store, projectID int64) (boardData, error) {
	statuses, err := st.ListStatuses(projectID)
	if err != nil {
		return boardData{}, err
	}
	issues, err := st.ListIssues(store.IssueFilter{ProjectID: projectID})
	if err != nil {
		return boardData{}, err
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
	return boardData{columns: cols, tags: tags}, nil
}
