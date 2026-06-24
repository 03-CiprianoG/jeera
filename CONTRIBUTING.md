# Contributing to Jeera

Thanks for your interest in Jeera! It's early, the foundations are still being
poured, and that makes it a great time to get involved.

## Ground rules

- Be kind and constructive — see the [Code of Conduct](CODE_OF_CONDUCT.md).
- `main` is **protected**: every change lands through a pull request, and CI
  must be green before it can be merged.

## Development setup

Prerequisites: **Go 1.26+**.

```sh
git clone https://github.com/03-CiprianoG/jeera.git
cd jeera
go build ./...
go test ./...
```

## Workflow

1. Create a branch off `main`:
   ```sh
   git switch -c feat/short-description
   ```
2. Make your change. Add tests where it makes sense.
3. Make sure everything passes locally:
   ```sh
   go build ./... && go vet ./... && go test ./...
   ```
4. Use [Conventional Commits](https://www.conventionalcommits.org/) for commit
   messages and PR titles (`feat:`, `fix:`, `docs:`, `refactor:`, `chore:` …).
5. Open a pull request, fill in the template, and link any related issues.
   Ensure CI is green.

## Project layout (planned)

```
cmd/        CLI entrypoints (root TUI, `mcp`, `version`)
internal/
  core/     domain model (Issue, Board, Status, Priority…)
  store/    persistence (local-first)
  tui/      Bubble Tea board
  mcp/      Model Context Protocol server
  agent/    pluggable AI layer (Claude-first)
```

## Reporting bugs & requesting features

Use the issue templates — they help us help you faster. For anything
security-related, please follow [SECURITY.md](SECURITY.md) instead of opening a
public issue.
