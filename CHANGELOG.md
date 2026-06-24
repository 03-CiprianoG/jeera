# Changelog

All notable changes to Jeera are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Core domain model** (`internal/core`): projects, issues (epic/story/task/bug/subtask),
  statuses with board categories, sprints, tags, issue links, comments, attachments,
  model-assignees (provider + model + reasoning effort), runs and schedules, with
  validation and issue-key formatting/parsing.
- **Local-first store** (`internal/store`): the system of record on a single SQLite
  database via the pure-Go `modernc.org/sqlite` driver (no CGO — the binary stays
  static), with embedded `goose` migrations, WAL journaling, enforced foreign keys,
  per-project monotonic issue keys, filtered issue listing, bidirectional issue links,
  and an in-process change-event bus so front-ends refresh on every committed write.
- **Command skeleton** (`main.go`): the `jeera` root command plus `--headless`,
  `--no-mcp`, `--version` and the `version` subcommand; XDG-aware path resolution
  (`internal/paths`) and build identity (`internal/version`).

[Unreleased]: https://github.com/03-CiprianoG/jeera/compare/main...HEAD
