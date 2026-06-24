package core

import "time"

// Project is a board bound to a code repository. Every issue, sprint, status,
// tag and run belongs to exactly one project. Jeera's store is global and can
// hold many projects; a run for one of a project's issues executes inside that
// project's RepoPath (or a git worktree of it).
type Project struct {
	ID        int64
	KeyPrefix string // uppercase, e.g. "JEE"; issue keys render as PREFIX-Seq
	Name      string
	RepoPath  string // absolute path to the project's git repository
	Defaults  ProjectDefaults
	CreatedAt time.Time
}

// ProjectDefaults holds the fallback run settings for a project's issues. A
// per-issue value (Assignee, WorktreeOn, Settings) overrides the matching
// default; a nil/zero field here falls back to the global config defaults.
type ProjectDefaults struct {
	Provider       Provider `json:"provider,omitempty"`
	Model          string   `json:"model,omitempty"`
	Effort         Effort   `json:"effort,omitempty"`
	WorktreeOn     *bool    `json:"worktree_on,omitempty"`
	PermissionMode string   `json:"permission_mode,omitempty"`
}

// Status is one named column on a project's board. Many statuses can share a
// StatusCategory; Position orders them within their category (and on the board).
type Status struct {
	ID        int64
	ProjectID int64
	Name      string
	Category  StatusCategory
	Position  int
}

// Assignee is the model that will execute an issue when it is run. Jeera
// assigns work to models, not people, so an assignee is a (provider, model,
// effort) triple rather than a username.
type Assignee struct {
	Provider Provider
	Model    string
	Effort   Effort
}

// IsZero reports whether no assignee has been set.
func (a Assignee) IsZero() bool {
	return a.Provider == "" && a.Model == "" && a.Effort == ""
}

// IssueSettings holds per-issue execution overrides that are not captured by
// the dedicated Issue fields. It is persisted as JSON so it can grow without a
// schema migration.
type IssueSettings struct {
	// PermissionMode overrides the provider permission posture for this issue's
	// runs (e.g. "bypassPermissions"). Empty inherits the project/global default.
	PermissionMode string `json:"permission_mode,omitempty"`
}

// Issue is a single work item: an epic, story, task, bug or subtask. Title and
// Description (Markdown) are the human-facing body; the remaining fields drive
// the board, the Jira-style relationships and Jeera's execution model.
type Issue struct {
	ID          int64
	ProjectID   int64
	Seq         int64  // per-project monotonic number; 0 until assigned by the store
	Key         string // PREFIX-Seq, derived from the project and Seq
	Type        IssueType
	Title       string
	Description string // Markdown
	StatusID    int64
	Priority    Priority
	StoryPoints *int     // nil when unestimated
	Assignee    Assignee // the model that runs this issue (may be zero)
	EpicID      *int64   // parent epic, for non-epic issues
	SprintID    *int64   // active/assigned sprint
	ParentID    *int64   // parent issue, for subtasks
	Rank        string   // lexicographic ordering key within a column/backlog
	WorktreeOn  *bool    // per-issue worktree override; nil inherits the default
	Settings    IssueSettings
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Sprint is a time-boxed set of issues within a project.
type Sprint struct {
	ID        int64
	ProjectID int64
	Name      string
	Goal      string
	State     SprintState
	StartAt   *time.Time
	EndAt     *time.Time
}

// Tag is a project-scoped label that can be attached to many issues.
type Tag struct {
	ID        int64
	ProjectID int64
	Name      string
	Color     string // hex like "#7C7CF0"; empty falls back to the theme default
}

// IssueLink is a directional relationship between two issues in the same
// project. A single stored edge can be shown from either side using
// LinkType.Inverse.
type IssueLink struct {
	ID        int64
	ProjectID int64
	SourceID  int64
	TargetID  int64
	Type      LinkType
}

// Comment is an activity entry on an issue. Author is "human" for the operator
// or "run:<id>" when an agent run posts via the MCP server.
type Comment struct {
	ID        int64
	IssueID   int64
	Author    string
	Body      string // Markdown
	CreatedAt time.Time
}

// Attachment references a file linked to an issue. Jeera stores the file path
// and metadata, never the binary contents, keeping the store small and the
// body diffable; rendering/opening is the front-end's job.
type Attachment struct {
	ID        int64
	IssueID   int64
	Filename  string
	Path      string
	MIME      string
	Size      int64
	CreatedAt time.Time
}

// Run is one execution of an issue by a provider CLI. Runs are versioned: a
// re-run or a fork of an earlier run creates a new Run with an incremented
// Version and a ParentRunID, leaving the original immutable.
type Run struct {
	ID             int64
	IssueID        int64
	Version        int
	ParentRunID    *int64
	Provider       Provider
	Model          string
	Effort         Effort
	SessionID      string // provider session/thread id, for resume and fork
	WorktreePath   string // empty when the run executes directly in the repo
	Branch         string
	Status         RunStatus
	PermissionMode string
	StartedAt      *time.Time
	EndedAt        *time.Time
	ExitCode       *int
	LogPath        string
}

// Schedule is a persisted "Schedule Start" entry: a cron specification that
// triggers a run of an issue while Jeera is running. JobUUID ties the row to a
// live scheduler job and is re-registered on startup.
type Schedule struct {
	ID           int64
	IssueID      int64
	CronSpec     string
	WithChildren bool
	Enabled      bool
	JobUUID      string
	NextRun      *time.Time
}
