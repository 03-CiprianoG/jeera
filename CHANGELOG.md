# Changelog

All notable changes to Jeera are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/03-CiprianoG/jeera/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/03-CiprianoG/jeera/releases/tag/v0.2.0
[0.1.0]: https://github.com/03-CiprianoG/jeera/releases/tag/v0.1.0
