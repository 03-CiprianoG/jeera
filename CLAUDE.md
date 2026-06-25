# CLAUDE.md

Guidance for Claude Code working in this repository. Keep it accurate and concise.

## What Jeera is
An agentic-first, terminal-native issue tracker — think *lazygit for issue tracking*. It is **local-first** and the **system of record**: Jeera owns issues in a local store on the user's machine.

## How it runs (important)
Running **`jeera`** starts **both** in a single process:
- the **TUI** (the human's board), and
- an embedded **MCP server over local HTTP** (Streamable HTTP) — stdio is taken by the terminal, so the server must be HTTP.

The user then points their MCP client (Claude Code, Claude Desktop, Cursor, …) at the server's URL **if and where they choose**. There is **no separate command** for the server. The TUI surfaces the MCP endpoint/status.

| Invocation | Result |
|---|---|
| `jeera` | TUI **+** MCP server (default) |
| `jeera --headless` | MCP server only (no TUI) |
| `jeera --no-mcp` | TUI only |
| `jeera version` | print version |

Both front-ends share one `core` model and one `store`, so the human and the agents never see different data.

## Stack
- **Go** — single static binary.
- **Bubble Tea v2** + **Lip Gloss v2** for the TUI. Vanity import paths: `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`. In v2, `View()` returns a `tea.View` struct (set `AltScreen: true` for full-screen); keys via `tea.KeyPressMsg.String()`.
- **MCP Go SDK** (`github.com/modelcontextprotocol/go-sdk`) for the agent server.
- **Claude-first** AI, behind a pluggable provider interface.

## Layout
```
main.go         CLI entry: root = TUI + MCP server; flags for --headless / --no-mcp / version
internal/
  core/         domain model (Issue, Status, Priority, Run, Schedule, Attachment, …) + validation
  store/        local-first SQLite persistence (system of record) + change-event bus
  mcp/          MCP HTTP server + 16 typed tools, over the shared store
  tui/          Bubble Tea board, ticket detail, runs/settings views, theme (the design system)
  agent/        pluggable provider drivers (claude/codex) — arg construction + stream parsing
  run/          execution engine: run manager, lifecycle, versioning, Start-with-children, Discuss
  worktree/     git worktree add/list/remove wrappers
  schedule/     gocron-backed persisted "Schedule Start"
  config/       global TOML config + the issue→project→global settings cascade
  paths/        XDG-aware data/config paths
  version/      build identity (stamped by GoReleaser)
```

## Common commands
```sh
go build ./...     # build
go vet ./...       # vet
go test ./...      # test
go run .           # run TUI + MCP server
```

## ⛔ MANDATORY engineering rules — no exceptions
1. **ALWAYS use the `frontend-design` skill** for any TUI/UI work (layout, components, styling, UX, theming). Never hand-roll UI without it.
2. **Build against real, official documentation.** Verify every external API before using it (`go doc <pkg>.<symbol>`, official docs/source). Never guess or rely on memory for library behavior.
3. **Write automated tests** for everything you add or change. Tests live beside the code as `_test.go`.
4. **Test every function, render, and output.** Unit-test all domain/store logic; snapshot/golden-test every TUI `View()` render; assert on every MCP tool's output. A change is **not done** until its behavior is proven by a passing test.

## Workflow
Two long-lived branches:
- **`main`** — the stable, released line. Every commit on `main` is a released version tagged `vX.Y.Z`; pushing the tag triggers GoReleaser (`.github/workflows/release.yml`). `main` is protected: PR-only, CI (`build`) must pass, **squash merges only**, linear history, no force-push. Never commit straight to `main`.
- **`dev`** — the integration branch where day-to-day work lands. Branch features off `dev` and merge them back via PR with **base `dev`** (`gh pr create --base dev`); CI runs on every PR.

**Cutting a release:** open a PR from `dev` → `main`, squash-merge it, then tag `main` with the new `vX.Y.Z` (semver: `feat` → minor, `fix` → patch, breaking → major) and push the tag to publish. Only ever tag a commit that is on `main`. Afterwards resync `dev` with the released `main` (`git switch dev && git merge main`) so the next release diff is clean.

- Use **Conventional Commits** (`feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `chore:`).
- Run `go build ./... && go vet ./... && go test ./...` before every commit.
