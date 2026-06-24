package agent

import (
	"encoding/json"
	"fmt"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// codexProvider drives the `codex` CLI in non-interactive exec mode with JSONL
// output. Codex mints its own thread id (read back from the first
// thread.started event) rather than accepting a pre-assigned one.
type codexProvider struct{}

func (codexProvider) Name() core.Provider     { return core.ProviderCodex }
func (codexProvider) Binary() string          { return "codex" }
func (codexProvider) PreassignsSession() bool { return false }

func (codexProvider) Args(spec RunSpec) []string {
	args := []string{
		"exec", "--json",
		"--skip-git-repo-check",
		"-a", "never",
		"-s", "workspace-write",
	}
	if spec.WorkDir != "" {
		args = append(args, "-C", spec.WorkDir)
	}
	if spec.Model != "" {
		args = append(args, "-m", spec.Model)
	}
	if spec.Effort != "" {
		// The -c value is parsed as TOML; a string needs quoting.
		args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", string(spec.Effort)))
	}
	if spec.MCPURL != "" {
		args = append(args, "-c", fmt.Sprintf("mcp_servers.jeera.url=%q", spec.MCPURL))
	}
	// The prompt is the trailing positional argument.
	args = append(args, spec.Prompt)
	return args
}

// codexLine mirrors the codex JSONL events we use.
type codexLine struct {
	Type     string `json:"type"`
	ThreadID string `json:"thread_id"`
	Item     struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"item"`
}

func (codexProvider) ParseLine(line []byte) (Event, bool) {
	var l codexLine
	if err := json.Unmarshal(line, &l); err != nil {
		return Event{}, false
	}
	switch l.Type {
	case "thread.started":
		if l.ThreadID != "" {
			return Event{Kind: EventSessionStarted, SessionID: l.ThreadID}, true
		}
	case "item.completed":
		if l.Item.Type == "agent_message" && l.Item.Text != "" {
			return Event{Kind: EventMessage, Text: l.Item.Text}, true
		}
	case "turn.completed":
		return Event{Kind: EventDone}, true
	}
	return Event{}, false
}
