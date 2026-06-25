// Package core defines Jeera's domain model: the projects, issues, sprints,
// runs and related value types shared by every front-end (the TUI and the MCP
// server) and by the local store that is Jeera's system of record.
//
// The types here are deliberately persistence- and transport-agnostic: they
// hold no SQL, no JSON-schema and no rendering logic, so that the store, the
// MCP layer and the TUI can each adapt them without the model depending on any
// of them.
package core

// Provider identifies a local AI coding CLI that Jeera can drive to execute a
// ticket. Jeera shells out to these tools directly — it never uses an SDK or an
// API key.
type Provider string

const (
	ProviderClaude Provider = "claude"
	ProviderCodex  Provider = "codex"
)

// Providers lists every supported provider in display order.
func Providers() []Provider { return []Provider{ProviderClaude, ProviderCodex} }

// Valid reports whether p is a known provider.
func (p Provider) Valid() bool {
	switch p {
	case ProviderClaude, ProviderCodex:
		return true
	}
	return false
}

// Effort is a reasoning-effort level passed to a provider. Not every provider
// supports every level (claude: low..max; codex: minimal..xhigh); the config
// layer validates an effort against the chosen provider and coerces where a
// provider does its own coercion.
type Effort string

const (
	EffortMinimal Effort = "minimal"
	EffortLow     Effort = "low"
	EffortMedium  Effort = "medium"
	EffortHigh    Effort = "high"
	EffortXHigh   Effort = "xhigh"
	EffortMax     Effort = "max"
)

// Efforts lists every effort level from least to most.
func Efforts() []Effort {
	return []Effort{EffortMinimal, EffortLow, EffortMedium, EffortHigh, EffortXHigh, EffortMax}
}

// Valid reports whether e is a known effort level.
func (e Effort) Valid() bool {
	switch e {
	case EffortMinimal, EffortLow, EffortMedium, EffortHigh, EffortXHigh, EffortMax:
		return true
	}
	return false
}

// IssueType is the kind of work item, following the familiar
// epic → story → task hierarchy.
type IssueType string

const (
	TypeEpic    IssueType = "epic"
	TypeStory   IssueType = "story"
	TypeTask    IssueType = "task"
	TypeBug     IssueType = "bug"
	TypeSubtask IssueType = "subtask"
)

// IssueTypes lists every issue type in display order.
func IssueTypes() []IssueType {
	return []IssueType{TypeEpic, TypeStory, TypeTask, TypeBug, TypeSubtask}
}

// Valid reports whether t is a known issue type.
func (t IssueType) Valid() bool {
	switch t {
	case TypeEpic, TypeStory, TypeTask, TypeBug, TypeSubtask:
		return true
	}
	return false
}

// Priority ranks an issue's urgency. The constant values are ordered from
// lowest to highest; use Rank for a numeric comparison.
type Priority string

const (
	PriorityLowest  Priority = "lowest"
	PriorityLow     Priority = "low"
	PriorityMedium  Priority = "medium"
	PriorityHigh    Priority = "high"
	PriorityHighest Priority = "highest"
)

// Priorities lists every priority from lowest to highest.
func Priorities() []Priority {
	return []Priority{PriorityLowest, PriorityLow, PriorityMedium, PriorityHigh, PriorityHighest}
}

// Valid reports whether p is a known priority.
func (p Priority) Valid() bool {
	switch p {
	case PriorityLowest, PriorityLow, PriorityMedium, PriorityHigh, PriorityHighest:
		return true
	}
	return false
}

// Rank returns a numeric ordering for the priority, 0 (lowest) to 4 (highest).
// Unknown priorities rank as -1.
func (p Priority) Rank() int {
	for i, pr := range Priorities() {
		if pr == p {
			return i
		}
	}
	return -1
}

// StatusCategory groups a project's statuses into the board lanes. A project may
// have many named statuses, but each maps to one category so the board, progress
// counts and the agent run-prompt have a stable vocabulary.
type StatusCategory string

const (
	CategoryTodo       StatusCategory = "todo"
	CategoryInProgress StatusCategory = "inprogress"
	CategoryReview     StatusCategory = "review"
	CategoryDone       StatusCategory = "done"
)

// StatusCategories lists the lanes left-to-right as they appear on the board.
func StatusCategories() []StatusCategory {
	return []StatusCategory{CategoryTodo, CategoryInProgress, CategoryReview, CategoryDone}
}

// Valid reports whether c is a known status category.
func (c StatusCategory) Valid() bool {
	switch c {
	case CategoryTodo, CategoryInProgress, CategoryReview, CategoryDone:
		return true
	}
	return false
}

// LinkType is the relationship between two issues. Links are directional;
// LinkBlocks from A→B means "A blocks B".
type LinkType string

const (
	LinkBlocks     LinkType = "blocks"
	LinkBlockedBy  LinkType = "blocked_by"
	LinkRelates    LinkType = "relates"
	LinkDuplicates LinkType = "duplicates"
)

// LinkTypes lists every link type in display order.
func LinkTypes() []LinkType {
	return []LinkType{LinkBlocks, LinkBlockedBy, LinkRelates, LinkDuplicates}
}

// Valid reports whether l is a known link type.
func (l LinkType) Valid() bool {
	switch l {
	case LinkBlocks, LinkBlockedBy, LinkRelates, LinkDuplicates:
		return true
	}
	return false
}

// Inverse returns the complementary link type, so a single stored edge can be
// presented from either issue's perspective. Relates and Duplicates are their
// own inverses.
func (l LinkType) Inverse() LinkType {
	switch l {
	case LinkBlocks:
		return LinkBlockedBy
	case LinkBlockedBy:
		return LinkBlocks
	default:
		return l
	}
}

// SprintState is the lifecycle stage of a sprint.
type SprintState string

const (
	SprintFuture    SprintState = "future"
	SprintActive    SprintState = "active"
	SprintCompleted SprintState = "completed"
)

// Valid reports whether s is a known sprint state.
func (s SprintState) Valid() bool {
	switch s {
	case SprintFuture, SprintActive, SprintCompleted:
		return true
	}
	return false
}

// RunStatus is the lifecycle stage of a single execution run.
type RunStatus string

const (
	RunQueued    RunStatus = "queued"
	RunRunning   RunStatus = "running"
	RunSucceeded RunStatus = "succeeded"
	RunFailed    RunStatus = "failed"
	RunCancelled RunStatus = "cancelled"
	RunBlocked   RunStatus = "blocked"
)

// Valid reports whether s is a known run status.
func (s RunStatus) Valid() bool {
	switch s {
	case RunQueued, RunRunning, RunSucceeded, RunFailed, RunCancelled, RunBlocked:
		return true
	}
	return false
}

// Terminal reports whether the run has reached a final state and will not
// transition further on its own.
func (s RunStatus) Terminal() bool {
	switch s {
	case RunSucceeded, RunFailed, RunCancelled:
		return true
	}
	return false
}
