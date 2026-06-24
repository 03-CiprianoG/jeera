package core

import "time"

// EventType names a change the store has just committed. The store publishes
// these so the TUI can refresh the moment an agent mutates data over the MCP
// server — the human's board and the agents never drift apart.
type EventType string

const (
	EventProjectChanged  EventType = "project.changed"
	EventIssueCreated    EventType = "issue.created"
	EventIssueUpdated    EventType = "issue.updated"
	EventIssueDeleted    EventType = "issue.deleted"
	EventCommentAdded    EventType = "comment.added"
	EventSprintChanged   EventType = "sprint.changed"
	EventRunChanged      EventType = "run.changed"
	EventScheduleChanged EventType = "schedule.changed"
)

// Event describes a single committed change. The IDs are populated when
// relevant to the EventType and are zero otherwise; subscribers should treat a
// received Event as a hint to re-read, not as the change payload itself.
type Event struct {
	Type      EventType
	ProjectID int64
	IssueID   int64
	RunID     int64
	At        time.Time
}
