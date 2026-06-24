package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/03-CiprianoG/jeera/internal/core"
)

func TestLoadMissingFileYieldsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("Load of a missing file should not error: %v", err)
	}
	want := Default()
	if cfg.Defaults != want.Defaults {
		t.Errorf("missing file should yield defaults: got %+v want %+v", cfg.Defaults, want.Defaults)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.toml")
	cfg := Default()
	cfg.Defaults.Provider = core.ProviderCodex
	cfg.Defaults.Model = "gpt-5.4"
	cfg.Defaults.Effort = core.EffortHigh
	cfg.Defaults.WorktreeOn = false
	cfg.MCPPort = 9000

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != cfg {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", got, cfg)
	}
}

// A partial file keeps built-in values for the fields it omits.
func TestLoadPartialOverlaysDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[defaults]\nmodel = \"sonnet\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Defaults.Model != "sonnet" {
		t.Errorf("model override not applied: %q", cfg.Defaults.Model)
	}
	// Untouched fields keep their defaults.
	if cfg.Defaults.Provider != core.ProviderClaude || !cfg.Defaults.WorktreeOn {
		t.Errorf("omitted fields should keep defaults: %+v", cfg.Defaults)
	}
}

func TestResolveRunIssueWins(t *testing.T) {
	global := Default().Defaults
	project := core.Project{Defaults: core.ProjectDefaults{Provider: core.ProviderCodex, Model: "gpt-5.4"}}
	issue := core.Issue{Assignee: core.Assignee{Provider: core.ProviderClaude, Model: "opus", Effort: core.EffortMax}}

	got := ResolveRun(global, project, issue)
	if got.Assignee != issue.Assignee {
		t.Errorf("issue assignee should win: got %+v", got.Assignee)
	}
}

func TestResolveRunProjectOverGlobal(t *testing.T) {
	global := Defaults{Provider: core.ProviderClaude, Model: "opus", Effort: core.EffortMedium, WorktreeOn: true, PermissionMode: "bypassPermissions"}
	wtOff := false
	project := core.Project{Defaults: core.ProjectDefaults{
		Provider: core.ProviderCodex, Model: "gpt-5.4", Effort: core.EffortHigh,
		WorktreeOn: &wtOff, PermissionMode: "plan",
	}}

	got := ResolveRun(global, project, core.Issue{})
	want := core.Assignee{Provider: core.ProviderCodex, Model: "gpt-5.4", Effort: core.EffortHigh}
	if got.Assignee != want {
		t.Errorf("project defaults should win over global: got %+v want %+v", got.Assignee, want)
	}
	if got.WorktreeOn {
		t.Error("project worktree=off should win over global on")
	}
	if got.PermissionMode != "plan" {
		t.Errorf("project permission should win: %q", got.PermissionMode)
	}
}

func TestResolveRunFallsBackToGlobal(t *testing.T) {
	global := Defaults{Provider: core.ProviderClaude, Model: "sonnet", Effort: core.EffortLow, WorktreeOn: true, PermissionMode: "bypassPermissions"}
	got := ResolveRun(global, core.Project{}, core.Issue{}) // empty project + issue
	want := core.Assignee{Provider: core.ProviderClaude, Model: "sonnet", Effort: core.EffortLow}
	if got.Assignee != want {
		t.Errorf("should fall back to global: got %+v want %+v", got.Assignee, want)
	}
	if !got.WorktreeOn || got.PermissionMode != "bypassPermissions" {
		t.Errorf("worktree/permission should come from global: %+v", got)
	}
}

// A model that doesn't belong to the resolved provider falls back to that
// provider's catalog default, so a run never launches with a mismatched model.
func TestResolveRunGuardsMismatchedModel(t *testing.T) {
	// Project forces codex but sets no model; global model is a Claude alias.
	global := Defaults{Provider: core.ProviderClaude, Model: "opus", Effort: core.EffortMedium}
	project := core.Project{Defaults: core.ProjectDefaults{Provider: core.ProviderCodex}}

	got := ResolveRun(global, project, core.Issue{})
	if got.Assignee.Provider != core.ProviderCodex {
		t.Fatalf("provider should be codex, got %v", got.Assignee.Provider)
	}
	wantModel := core.DefaultAssignee(core.ProviderCodex).Model
	if got.Assignee.Model != wantModel {
		t.Errorf("mismatched model should fall back to codex default %q, got %q", wantModel, got.Assignee.Model)
	}
}

// An issue-level assignee with a model that doesn't belong to its provider — as
// can be persisted via the MCP set_assignee tool — is corrected to the provider's
// default rather than launched as-is.
func TestResolveRunGuardsIssueAssigneeModel(t *testing.T) {
	global := Default().Defaults
	issue := core.Issue{Assignee: core.Assignee{Provider: core.ProviderClaude, Model: "gpt-5.4", Effort: core.EffortHigh}}
	got := ResolveRun(global, core.Project{}, issue)
	if got.Assignee.Provider != core.ProviderClaude {
		t.Fatalf("provider should stay claude, got %v", got.Assignee.Provider)
	}
	want := core.DefaultAssignee(core.ProviderClaude).Model
	if got.Assignee.Model != want {
		t.Errorf("mismatched issue model should fall back to %q, got %q", want, got.Assignee.Model)
	}
}

// Effort is validated against the resolved provider, so a cross-layer combination
// like claude + "minimal" (a codex-only effort) is corrected.
func TestResolveRunGuardsCrossLayerEffort(t *testing.T) {
	// Project forces claude; global carries a codex-only effort.
	global := Defaults{Provider: core.ProviderCodex, Model: "gpt-5.4", Effort: core.EffortMinimal}
	project := core.Project{Defaults: core.ProjectDefaults{Provider: core.ProviderClaude, Model: "opus"}}
	got := ResolveRun(global, project, core.Issue{})
	if got.Assignee.Provider != core.ProviderClaude {
		t.Fatalf("provider should be claude, got %v", got.Assignee.Provider)
	}
	if core.SupportsEffort(core.ProviderClaude, got.Assignee.Effort) == false {
		t.Errorf("effort %q is not valid for claude; guard should have corrected it", got.Assignee.Effort)
	}
	if got.Assignee.Effort != core.DefaultAssignee(core.ProviderClaude).Effort {
		t.Errorf("effort should fall back to the provider default, got %q", got.Assignee.Effort)
	}
}

func TestResolveRunIssuePermissionWins(t *testing.T) {
	global := Defaults{Provider: core.ProviderClaude, Model: "opus", Effort: core.EffortMedium, PermissionMode: "bypassPermissions"}
	project := core.Project{Defaults: core.ProjectDefaults{PermissionMode: "plan"}}
	issue := core.Issue{Settings: core.IssueSettings{PermissionMode: "acceptEdits"}}
	if got := ResolveRun(global, project, issue); got.PermissionMode != "acceptEdits" {
		t.Errorf("issue permission mode should win the cascade, got %q", got.PermissionMode)
	}
}

// On a disk-write failure, Save must not swap in the new config — the live value
// stays as it was so a bad write can't silently change behavior.
func TestStoreSaveFailureKeepsLiveConfig(t *testing.T) {
	// A path whose parent is a regular file can't be created.
	notADir := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// NewStore may report a read error for this path, but still returns a usable
	// Store holding the built-in defaults.
	st, _ := NewStore(filepath.Join(notADir, "config.toml"))
	before := st.Get()

	bad := Default()
	bad.Defaults.Model = "changed"
	if err := st.Save(bad); err == nil {
		t.Fatal("Save to an uncreatable path should error")
	}
	if st.Get() != before {
		t.Errorf("a failed Save must not change the live config: got %+v want %+v", st.Get(), before)
	}
}

func TestResolveRunIssueWorktreeOverridesProject(t *testing.T) {
	on := true
	off := false
	global := Default().Defaults
	project := core.Project{Defaults: core.ProjectDefaults{WorktreeOn: &off}}
	issue := core.Issue{WorktreeOn: &on}
	if !ResolveRun(global, project, issue).WorktreeOn {
		t.Error("issue worktree=on should override project off")
	}
}
