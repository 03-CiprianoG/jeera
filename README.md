<div align="center">

# Jeera

**Agentic-first issue tracking that lives in your terminal.**

A [lazygit](https://github.com/jesseduffield/lazygit)-inspired TUI that reimagines Jira for the age of AI agents — with a built-in [MCP](https://modelcontextprotocol.io) server so your agents always know about your tickets.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![CI](https://github.com/03-CiprianoG/jeera/actions/workflows/ci.yml/badge.svg)](https://github.com/03-CiprianoG/jeera/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![Status](https://img.shields.io/badge/status-pre--alpha-orange)

</div>

> [!WARNING]
> **🚧 Early days.** This repository is being set up and Jeera is being built in the open. The architecture below is settled; the implementation lands incrementally via pull requests. Expect rapid change — and feel free to get involved.

## What is Jeera?

Jeera is an open-source issue tracker that runs entirely in your terminal. Think of the keyboard-driven flow of `lazygit`, applied to issues, sprints and boards — but designed from the first commit for a world where **AI agents are first-class operators**, not bystanders.

Jeera is **local-first** and the **system of record**: it owns your tickets in a local store on your machine. A human drives the board through a fast, beautiful TUI; agents drive the same tickets through a built-in **Model Context Protocol server**. Both read and write one source of truth, so they never drift apart.

## Why "agentic-first"?

Most tools bolt a chat box onto a GUI. Jeera inverts that. The agent isn't a feature *inside* the app — the app is a clean surface that *agents connect to*:

- Run `jeera` and you get a snappy terminal board for humans.
- Run `jeera mcp` and any MCP client (Claude Code, Claude Desktop, Cursor, a cron agent…) can **list, create, transition, and comment on issues** — instantly aware of every ticket, with no scraping and no glue code.

That means an agent can triage your backlog while you sleep, file issues from a failing CI run, or keep a board in sync with your codebase — all through a typed, documented protocol.

## Planned features

- ⌨️ **Keyboard-first Kanban board** — vim-style navigation across backlog → done.
- 🤖 **Built-in MCP server** (`jeera mcp`) — typed tools for agents over stdio or HTTP.
- 🗃️ **Local-first system of record** — your tickets live on your machine; no account required.
- 🧠 **Claude-first, pluggable AI layer** — assistive features (e.g. `jeera triage`) behind a provider interface.
- 📦 **Single static binary** — `go install` or grab a release; no runtime to manage.
- 🔁 **Optional Jira Cloud import/sync** *(later)* — bring an existing project in.

## Architecture

One binary, two front-ends, one source of truth:

```
                  ┌─────────────────────────────────┐
   You  ────────► │              jeera              │ ◄──────── AI agents
  (keyboard)      │                                 │       (Claude Code,
                  │   ┌──────────┐   ┌───────────┐  │        Cursor, cron…)
                  │   │   TUI    │   │    MCP     │  │
                  │   │ Bubble   │   │  server    │  │   via the Model
                  │   │  Tea v2  │   │ `jeera mcp`│  │   Context Protocol
                  │   └────┬─────┘   └─────┬──────┘  │
                  │        └───────┬───────┘         │
                  │            core + store          │
                  │        (one source of truth)     │
                  └─────────────────────────────────┘
```

## Stack

Chosen deliberately, not by default:

| Layer | Choice | Why |
|-------|--------|-----|
| Language | **Go** | Single static binary, trivial cross-compilation, great concurrency, large contributor pool |
| TUI | **Bubble Tea v2** + **Lip Gloss** | The gold standard for clean, sophisticated terminal UIs; v2's renderer is built for speed |
| Agents | **MCP Go SDK** | Official, GA, lets the same binary serve agents and humans |
| AI | **Claude-first**, pluggable | Agentic features with a provider interface for others later |

## Roadmap

- [x] **M0** — Repository & open-source project setup *(you are here)*
- [ ] **M1** — Domain model + local store
- [ ] **M2** — Kanban board TUI
- [ ] **M3** — MCP server (list / create / transition / comment)
- [ ] **M4** — Claude-powered backlog triage
- [ ] **M5** — Optional Jira Cloud import

## Contributing

Contributions are very welcome — especially this early, when the foundations are still being poured. See **[CONTRIBUTING.md](CONTRIBUTING.md)**. In short: `main` is protected, so all changes come in via pull request and must pass CI.

By participating you agree to our [Code of Conduct](CODE_OF_CONDUCT.md).

## License

[MIT](LICENSE) © 2026 Giuseppe Cipriano and the Jeera contributors.

## Acknowledgements

Standing on the shoulders of [Charm](https://charm.sh) (Bubble Tea & Lip Gloss), the [Model Context Protocol](https://modelcontextprotocol.io), and [lazygit](https://github.com/jesseduffield/lazygit) for the inspiration.
