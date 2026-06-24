package agent

import (
	"slices"
	"strings"
	"testing"

	"github.com/03-CiprianoG/jeera/internal/core"
)

func argString(args []string) string { return strings.Join(args, " ") }

func TestClaudeArgs(t *testing.T) {
	p := For(core.ProviderClaude)
	if p == nil || p.Binary() != "claude" || !p.PreassignsSession() {
		t.Fatalf("unexpected claude provider: %v", p)
	}
	args := p.Args(RunSpec{
		Prompt:        "do it",
		Model:         "opus",
		Effort:        core.EffortHigh,
		SessionID:     "abc-123",
		MCPConfigPath: "/tmp/jeera.json",
	})
	got := argString(args)
	for _, want := range []string{
		"-p do it",
		"--output-format stream-json",
		"--verbose",
		"--strict-mcp-config",
		"--mcp-config /tmp/jeera.json",
		"--model opus",
		"--effort high",
		"--session-id abc-123",
		"--permission-mode bypassPermissions", // default
	} {
		if !strings.Contains(got, want) {
			t.Errorf("claude args missing %q in:\n%s", want, got)
		}
	}
	// --bare must NOT be present (it would detach the run from Jeera's MCP).
	if slices.Contains(args, "--bare") {
		t.Error("claude args must not include --bare")
	}
}

func TestClaudeArgsCustomPermission(t *testing.T) {
	args := For(core.ProviderClaude).Args(RunSpec{Prompt: "x", PermissionMode: "dontAsk"})
	if !strings.Contains(argString(args), "--permission-mode dontAsk") {
		t.Errorf("custom permission mode not honored: %v", args)
	}
}

func TestCodexArgs(t *testing.T) {
	p := For(core.ProviderCodex)
	if p == nil || p.Binary() != "codex" || p.PreassignsSession() {
		t.Fatalf("unexpected codex provider: %v", p)
	}
	args := p.Args(RunSpec{
		Prompt:  "ship it",
		Model:   "gpt-5.4",
		Effort:  core.EffortMedium,
		WorkDir: "/repo/wt",
		MCPURL:  "http://127.0.0.1:7777",
	})
	got := argString(args)
	for _, want := range []string{
		"exec --json",
		"--skip-git-repo-check",
		"-a never",
		"-s workspace-write",
		"-C /repo/wt",
		"-m gpt-5.4",
		`model_reasoning_effort="medium"`,
		`mcp_servers.jeera.url="http://127.0.0.1:7777"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("codex args missing %q in:\n%s", want, got)
		}
	}
	// The prompt must be the final positional argument.
	if args[len(args)-1] != "ship it" {
		t.Errorf("prompt should be last arg, got %q", args[len(args)-1])
	}
}

func TestClaudeParseLine(t *testing.T) {
	p := For(core.ProviderClaude)
	cases := []struct {
		line string
		kind EventKind
		want string // session id or text or result
	}{
		{`{"type":"system","subtype":"init","session_id":"495e3558-780b"}`, EventSessionStarted, "495e3558-780b"},
		{`{"type":"assistant","message":{"content":[{"type":"text","text":"working on it"}]}}`, EventMessage, "working on it"},
		{`{"type":"result","session_id":"495e3558-780b","result":"DONE","is_error":false}`, EventDone, "DONE"},
		{`not json`, EventOther, ""},
		{`{"type":"user"}`, EventOther, ""},
	}
	for _, c := range cases {
		ev, ok := p.ParseLine([]byte(c.line))
		if c.kind == EventOther {
			if ok {
				t.Errorf("expected no event for %q, got %+v", c.line, ev)
			}
			continue
		}
		if !ok || ev.Kind != c.kind {
			t.Errorf("ParseLine(%q) kind = %v (ok=%v), want %v", c.line, ev.Kind, ok, c.kind)
			continue
		}
		switch c.kind {
		case EventSessionStarted:
			if ev.SessionID != c.want {
				t.Errorf("session id = %q, want %q", ev.SessionID, c.want)
			}
		case EventMessage:
			if ev.Text != c.want {
				t.Errorf("text = %q, want %q", ev.Text, c.want)
			}
		case EventDone:
			if ev.Result != c.want {
				t.Errorf("result = %q, want %q", ev.Result, c.want)
			}
		}
	}
}

func TestCodexParseLine(t *testing.T) {
	p := For(core.ProviderCodex)
	ev, ok := p.ParseLine([]byte(`{"type":"thread.started","thread_id":"019ef7c4-8b8f"}`))
	if !ok || ev.Kind != EventSessionStarted || ev.SessionID != "019ef7c4-8b8f" {
		t.Errorf("thread.started parse = %+v (ok=%v)", ev, ok)
	}
	ev, ok = p.ParseLine([]byte(`{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"OK"}}`))
	if !ok || ev.Kind != EventMessage || ev.Text != "OK" {
		t.Errorf("item.completed parse = %+v (ok=%v)", ev, ok)
	}
	ev, ok = p.ParseLine([]byte(`{"type":"turn.completed","usage":{}}`))
	if !ok || ev.Kind != EventDone {
		t.Errorf("turn.completed parse = %+v (ok=%v)", ev, ok)
	}
	if _, ok := p.ParseLine([]byte(`{"type":"turn.started"}`)); ok {
		t.Error("turn.started should not produce an event")
	}
	// An item.completed for a non-message item (a command, reasoning, etc.) must
	// not be mistaken for assistant text.
	if _, ok := p.ParseLine([]byte(`{"type":"item.completed","item":{"type":"command_execution","text":"ls"}}`)); ok {
		t.Error("a non-agent_message item.completed should not produce an event")
	}
	// An agent_message with no text carries nothing to show.
	if _, ok := p.ParseLine([]byte(`{"type":"item.completed","item":{"type":"agent_message","text":""}}`)); ok {
		t.Error("an empty agent_message should not produce an event")
	}
}

func TestDiscussPrompt(t *testing.T) {
	p := DiscussPrompt("JEE-12")
	for _, want := range []string{"JEE-12", "jeera.get_issue", "Don't start implementing"} {
		if !strings.Contains(p, want) {
			t.Errorf("discuss prompt missing %q in:\n%s", want, p)
		}
	}
}

func TestForUnknown(t *testing.T) {
	if For("gemini") != nil {
		t.Error("unknown provider should return nil")
	}
}

func TestRunPrompt(t *testing.T) {
	p := RunPrompt("JEE-12", "Build the board")
	for _, want := range []string{"JEE-12", "Build the board", "jeera.get_issue", "jeera.transition_issue", "In Progress", "Done"} {
		if !strings.Contains(p, want) {
			t.Errorf("run prompt missing %q", want)
		}
	}
}
