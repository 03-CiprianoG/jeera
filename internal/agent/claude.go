package agent

import (
	"encoding/json"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// claudeProvider drives the `claude` CLI in print mode with streaming JSON
// output. It can pre-assign the session id (so a run is resumable/forkable) and
// is pointed at exactly Jeera's MCP server via --mcp-config + --strict-mcp-config.
//
// Note: --bare is intentionally NOT used. On this CLI, --bare skips --mcp-config,
// which would detach the run from Jeera's MCP. --strict-mcp-config gives the
// reproducibility we want (only the provided server is loaded) without that.
type claudeProvider struct{}

func (claudeProvider) Name() core.Provider     { return core.ProviderClaude }
func (claudeProvider) Binary() string          { return "claude" }
func (claudeProvider) PreassignsSession() bool { return true }

func (claudeProvider) Args(spec RunSpec) []string {
	args := []string{
		"-p", spec.Prompt,
		"--output-format", "stream-json",
		"--verbose",
		"--strict-mcp-config",
	}
	if spec.MCPConfigPath != "" {
		args = append(args, "--mcp-config", spec.MCPConfigPath)
	}
	if spec.Model != "" {
		args = append(args, "--model", spec.Model)
	}
	if spec.Effort != "" {
		args = append(args, "--effort", string(spec.Effort))
	}
	if spec.SessionID != "" {
		args = append(args, "--session-id", spec.SessionID)
	}
	mode := spec.PermissionMode
	if mode == "" {
		mode = "bypassPermissions"
	}
	args = append(args, "--permission-mode", mode)
	return args
}

// ResumeArgs re-opens a claude session interactively by its id. `--resume <id>`
// (the long form of `-r`) drops the user back into the conversation; with no -p
// it runs as a normal interactive session in whatever terminal launches it.
func (claudeProvider) ResumeArgs(sessionID string) []string {
	return []string{"--resume", sessionID}
}

// claudeLine mirrors the fields of claude's stream-json events that we use.
type claudeLine struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	Result    string `json:"result"`
	IsError   bool   `json:"is_error"`
	Message   struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
}

func (claudeProvider) ParseLine(line []byte) (Event, bool) {
	var l claudeLine
	if err := json.Unmarshal(line, &l); err != nil {
		return Event{}, false
	}
	switch l.Type {
	case "system":
		if l.SessionID != "" {
			return Event{Kind: EventSessionStarted, SessionID: l.SessionID}, true
		}
	case "assistant":
		var text string
		for _, c := range l.Message.Content {
			if c.Type == "text" {
				text += c.Text
			}
		}
		if text != "" {
			return Event{Kind: EventMessage, Text: text}, true
		}
	case "result":
		return Event{Kind: EventDone, Result: l.Result, SessionID: l.SessionID, IsError: l.IsError}, true
	}
	return Event{}, false
}
