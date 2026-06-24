// Package agent drives the locally-installed AI coding CLIs (claude, codex) to
// execute a Jeera ticket. It deliberately uses no SDK and no API key — it builds
// a command line for the chosen provider, and parses the provider's streaming
// output into a small set of events. The run manager (internal/run) owns the
// process lifecycle; this package owns the provider-specific command shape and
// event parsing, so both are unit-testable without spawning anything.
package agent

import (
	"github.com/03-CiprianoG/jeera/internal/core"
)

// RunSpec is everything a provider needs to build a single run's command.
type RunSpec struct {
	Prompt         string
	Model          string
	Effort         core.Effort
	SessionID      string // pre-assigned (claude); ignored by codex, which mints its own
	WorkDir        string // working directory for the run (a repo or a worktree)
	MCPConfigPath  string // path to the Jeera MCP client config JSON (claude)
	MCPURL         string // the Jeera MCP URL (codex wires it via -c)
	PermissionMode string // claude permission posture; defaults to bypassPermissions
}

// EventKind classifies a parsed line of provider output.
type EventKind int

const (
	// EventOther is a line that carries no event we model.
	EventOther EventKind = iota
	// EventSessionStarted carries the provider session/thread id.
	EventSessionStarted
	// EventMessage carries assistant/progress text.
	EventMessage
	// EventDone marks the end of the run, with the final result.
	EventDone
)

// Event is a provider-agnostic view of one output line.
type Event struct {
	Kind      EventKind
	SessionID string // EventSessionStarted
	Text      string // EventMessage
	Result    string // EventDone
	IsError   bool   // EventDone
}

// Provider builds a run command for one AI CLI and parses its output stream.
type Provider interface {
	// Name is the provider this driver speaks for.
	Name() core.Provider
	// Binary is the executable to invoke.
	Binary() string
	// Args returns the command-line arguments (excluding the binary) for a run.
	Args(spec RunSpec) []string
	// PreassignsSession reports whether SessionID may be set before the run
	// starts (claude) or must be read back from the output (codex).
	PreassignsSession() bool
	// ParseLine extracts an Event from one line of streaming output. ok is false
	// for lines that carry no modeled event.
	ParseLine(line []byte) (ev Event, ok bool)
}

// For returns the driver for a provider, or nil if unknown.
func For(p core.Provider) Provider {
	switch p {
	case core.ProviderClaude:
		return claudeProvider{}
	case core.ProviderCodex:
		return codexProvider{}
	}
	return nil
}
