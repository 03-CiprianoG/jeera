# Changelog

All notable changes to Jeera are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.6.0] - 2026-06-27

Sprints get a full-screen detail with a burndown, and the board becomes a true
SCRUM board scoped to the active sprint.

### Added
- **Sprint detail view** (`internal/tui`): `enter` on a sprint opens a full-screen
  bento — the **Goal** as the hero (editable in place), a hand-rolled braille
  **burndown** (the ideal guideline vs. the work *actually* remaining, reconstructed
  from each done issue's last-updated day), a **Progress** readout (percent ·
  points/issues · a pace verdict — *on track / behind / ahead*), a status
  **Breakdown** with per-category bars, and the sprint's **Issues** — drill into any
  one with `enter`, `esc` back. Start/finish/reopen and date editing run through the
  same SCRUM-safe store methods as the Sprints list. Metrics are a pure, unit-tested
  function; six golden renders cover the active/future/completed/issue-basis/empty/
  narrow states.

### Changed
- **The board is scoped to the active sprint** (#22): it shows the active sprint's
  issues across *To Do / In Progress / In Review / Done*; with no active sprint it
  prompts you to start one, keeping the board and the SCRUM cadence in lockstep.

## [1.0.0] - 2026-06-24

Feature-complete and stable. Jeera is the human's board and the agents' MCP
server in one local-first binary, and a ticket is something you can run, schedule,
discuss, and attach to — backed by a settings cascade and per-run git worktrees.

### Added
- **Release engineering**: a GoReleaser pipeline builds cross-platform static
  binaries (linux/darwin/windows × amd64/arm64, CGO-free) with the version, commit
  and date stamped in, and a tag-triggered release workflow attaches them — with
  checksums — to the GitHub release. `go install github.com/03-CiprianoG/jeera@latest`
  is supported.

### Changed
- Documentation finalized: install via pre-built binary, `go install`, or source; a
  configuration reference; and the feature matrix marked complete.

## [0.7.0] - 2026-06-24

Attachments: pin links and files to a ticket and open them in a keystroke.

### Added
- **Attachments** (`internal/core`, `internal/store`): attach a URL or a file path
  to a ticket. Jeera stores the reference and metadata (filename, MIME, size) —
  never the binary — so the store stays small and the body diffable. A reference is
  classified as a link or a file automatically; file paths are absolutized so they
  open from anywhere.
- **In the ticket detail view**: `A` adds an attachment (URL or path), the sidebar
  lists them (🔗 links, 📎 files), and `o` opens the most recent one in your default
  app or browser (`xdg-open` / `open` / `start`).
- **`add_attachment` MCP tool**: agents can attach links and files to the ticket
  they're working — the 16th typed tool.

### Security
- The external "open" path validates a reference before handing it to the OS: only
  http/https URLs and existing regular local files are opened, so an agent-supplied
  attachment can't dispatch a `file://` / `javascript:` / `smb://` scheme or a UNC
  path. On Windows the opener uses `rundll32` rather than `cmd /c start`, so a ref is
  never re-parsed by a shell.

## [0.6.0] - 2026-06-24

Two more ways to put an agent to work on a ticket: talk it through, or run a
whole tree of work in order.

### Added
- **Expand / Discuss** (`d` in the ticket detail view): suspend the board and drop
  into an interactive `claude` session with Jeera's MCP attached and the ticket
  preloaded — to clarify scope, acceptance criteria and approach before any code is
  written. On exit the board resumes and reloads the (possibly updated) ticket.
- **Start with children** (`D` in the ticket detail view): run a ticket's child
  issues first — in **dependency order**, so a blocker runs before what it blocks —
  and then the ticket itself, each run finishing before the next begins. A
  dependency cycle degrades gracefully rather than hanging.

### Changed
- The run manager now has a lifecycle context: shutting Jeera down cancels every
  in-flight run and stops a sequenced "Start with children" from beginning further
  children.

## [0.5.0] - 2026-06-24

Settings & the configuration cascade: set your defaults once, override them where
it matters, and point projects at any repo.

### Added
- **Settings cascade** (`internal/config`): run settings now resolve through three
  layers — the issue's own values, then the project's defaults, then a global
  config file — so "Start" behaves predictably whether a ticket was filled in
  fully, partially, or not at all. A model that doesn't belong to the resolved
  provider falls back to that provider's catalog default, so a run never launches
  with a mismatched model.
- **Global config file** (`~/.config/jeera/config.toml`): a TOML file holding the
  fallback provider/model/effort, worktree default, permission mode and MCP port.
  Missing or partial files fall back to built-in defaults; writes are atomic.
- **Settings view** (`internal/tui/settings.go`): press `,` to edit the global
  defaults in place (`j/k` to move, `h/l` to change). Every change is saved
  immediately and picked up live by the run manager and scheduler.
- **Configurable MCP port**: `mcp_port` in the config file (and the existing
  `JEERA_MCP_PORT`, which still wins) sets the preferred port.
- **Project repo path on create**: the New Project form now takes the repository
  path (pre-filled with the current directory), and the projects switcher shows
  each project's repo — making "point Jeera at a repo" explicit.

## [0.4.0] - 2026-06-24

Scheduling: a ticket can now start itself on a cron. Set it once and walk away —
Jeera runs the agent on time, even headless.

### Added
- **Schedule Start** (`internal/schedule`): from the ticket detail view, press `S`
  and enter a cron spec (`0 9 * * *`) to have Jeera run the ticket on that
  schedule. Schedules are persisted in the store and **re-registered on boot**, so
  they survive restarts; `X` removes the most recent one. Each firing takes the
  same path as a manual Start, so a scheduled run is just an automated one.
- **gocron-backed scheduler**: a thin lifecycle layer over
  `github.com/go-co-op/gocron/v2` — register/unregister jobs live, persist each
  schedule's next-run time, and skip (and disable) any schedule whose spec no
  longer parses rather than wedging startup. A 6-field spec is read as
  second-resolution.
- **Headless scheduling** (`jeera --headless`): the execution engine and scheduler
  run without the TUI, so a quiet machine works its backlog on time. The headless
  banner reports how many schedules are enabled.
- **Schedules in the sidebar**: the ticket detail view lists a ticket's schedules
  with their cron spec and next-run time.

## [0.3.0] - 2026-06-24

The execution engine: a ticket isn't just something you track — press **Start**
and Jeera puts a real coding agent to work on it.

### Added
- **Execution engine** (`internal/run`): press `s` on a ticket to spawn the
  assignee's CLI (`claude`/`codex`) as a background process that actually does the
  work. The run is pointed back at Jeera's own MCP server, so the agent moves the
  very ticket it's running — To Do → In Progress → Done — and the board reflects it
  live. Output streams to a per-run log; the session id and final status are
  recorded.
- **Provider drivers** (`internal/agent`): a pluggable `Provider` interface with
  Claude-first `claude` and `codex` drivers — exact CLI argument construction,
  stream-json / JSONL event parsing, and session-id capture (claude pre-assigns a
  `--session-id`; codex's thread id is read from its first event). No API keys, no
  SDKs — it drives the CLIs already on your machine.
- **Per-ticket git worktrees** (`internal/worktree`): each run is isolated on its
  own `jeera/<key>-v<n>` branch in a dedicated worktree by default (`git worktree`,
  never `rm -rf`); toggle it off with `w` to run directly in the repo.
- **Run versioning** (`internal/store/runs.go`): every Start is a new, monotonic
  run version on the ticket, recorded with provider/model/effort, worktree, branch,
  session id, status, and exit code.
- **Runs view** (`internal/tui/runs.go`): press `R` for the global run list —
  active and recent runs with their ticket, version and live status; the ticket
  detail sidebar shows the worktree state and a ticket's recent runs.
- **Concise run prompt** (`internal/agent/prompt.go`): a tight, reliable template
  that tells the agent to read the ticket over MCP, transition it, implement and
  verify the work, then comment and close it (or mark it Blocked).

### Verified
- A real end-to-end run (`internal/run/e2e_test.go`, build-tag `e2e`): a live
  `claude` agent spawned in a worktree, connected to Jeera's MCP, drove a ticket
  To Do → In Progress → Done, created and committed a file, and left a comment.

## [0.2.0] - 2026-06-24

The ticket detail view: open any card to read and edit the full issue.

### Added
- **Ticket detail view** (`internal/tui/detail.go`): press Enter on a card to open
  a full-screen ticket. A Glamour-rendered, scrollable Markdown description on the
  left; an editable metadata sidebar on the right — Status, Type, Priority, Story
  Points, Assignee (Provider · Model · Effort), Sprint, Epic and Tags, each cycled
  in place with `h`/`l`; the activity timeline below.
- **Inline editing**: `e` edits the description in a textarea (`ctrl+s` to save);
  `c` adds a comment; Points and Tags via a prompt; `x` removes a tag. Every edit
  persists to the store immediately and the view reloads, staying consistent with
  concurrent agent changes over MCP.
- **Model/effort catalog** (`internal/core/catalog.go`): the selectable models and
  reasoning-effort levels per provider, powering the assignee picker.

### Fixed
- Cycling the Provider from unassigned now selects the first provider instead of
  skipping it; cycling Model/Effort from an out-of-catalog value lands on the
  first catalog entry rather than skipping it.

## [0.1.0] - 2026-06-24

First runnable release: a keyboard-driven kanban board and an embedded MCP
server, both backed by one local store.

### Added
- **Core domain model** (`internal/core`): projects, issues (epic/story/task/bug/subtask),
  statuses with board categories, sprints, tags, issue links, comments, attachments,
  model-assignees (provider + model + reasoning effort), runs and schedules, with
  validation and issue-key formatting/parsing.
- **Local-first store** (`internal/store`): the system of record on a single SQLite
  database via the pure-Go `modernc.org/sqlite` driver (no CGO — the binary stays
  static), with embedded `goose` migrations, WAL journaling, enforced foreign keys,
  per-project monotonic issue keys, filtered issue listing, bidirectional issue links,
  cross-project integrity guards, and an in-process change-event bus.
- **Embedded MCP server** (`internal/mcp`): an MCP server over local HTTP
  (Streamable HTTP, via the official Go SDK) exposing 15 typed tools over the shared
  store — `create_project`, `list_projects`, `get_project`, `list_issues`, `get_issue`,
  `create_issue`, `update_issue`, `transition_issue`, `set_assignee`, `add_comment`,
  `link_issues`, `list_sprints`, `add_to_sprint`, `list_tags`, `tag_issue`. Binds
  loopback with port fallback, logs nothing to the terminal, and emits a copy-paste
  client config.
- **TUI** (`internal/tui`): a Bubble Tea v2 kanban board with the "Slate & Iris" design
  system — status columns, model-assignee cards, an always-on MCP status pill, vim-style
  navigation, create/rename/delete, move-card-across-columns, a projects switcher, a help
  overlay, and live refresh when an agent writes over MCP.
- **Command surface** (`main.go`): `jeera` (board + MCP server), `jeera --headless`
  (MCP only), `jeera --no-mcp` (board only), `jeera version`; XDG-aware paths
  (`internal/paths`) and build identity (`internal/version`).

[Unreleased]: https://github.com/03-CiprianoG/jeera/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/03-CiprianoG/jeera/releases/tag/v1.0.0
[0.7.0]: https://github.com/03-CiprianoG/jeera/releases/tag/v0.7.0
[0.6.0]: https://github.com/03-CiprianoG/jeera/releases/tag/v0.6.0
[0.5.0]: https://github.com/03-CiprianoG/jeera/releases/tag/v0.5.0
[0.4.0]: https://github.com/03-CiprianoG/jeera/releases/tag/v0.4.0
[0.3.0]: https://github.com/03-CiprianoG/jeera/releases/tag/v0.3.0
[0.2.0]: https://github.com/03-CiprianoG/jeera/releases/tag/v0.2.0
[0.1.0]: https://github.com/03-CiprianoG/jeera/releases/tag/v0.1.0
