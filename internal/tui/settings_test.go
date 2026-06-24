package tui

import (
	"path/filepath"
	"testing"

	"github.com/03-CiprianoG/jeera/internal/config"
	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

func newSettingsForTest(t *testing.T) (*settingsModel, *config.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := config.NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return newSettings(cfg, theme.New(), 100, 30), cfg, path
}

func TestSettingsCycleProviderPersists(t *testing.T) {
	s, cfg, path := newSettingsForTest(t)
	// Default provider is claude; cycling lands on codex and keeps the model valid.
	s.field = sfProvider
	s.cycle(+1)

	if s.draft.Defaults.Provider != core.ProviderCodex {
		t.Errorf("provider did not cycle to codex: %v", s.draft.Defaults.Provider)
	}
	if !containsStr(core.ProviderModels(core.ProviderCodex), s.draft.Defaults.Model) {
		t.Errorf("model should be valid for codex, got %q", s.draft.Defaults.Model)
	}
	// Live store updated…
	if cfg.Get().Defaults.Provider != core.ProviderCodex {
		t.Error("change not swapped into the live store")
	}
	// …and persisted to disk.
	reloaded, _ := config.Load(path)
	if reloaded.Defaults.Provider != core.ProviderCodex {
		t.Errorf("change not persisted: %+v", reloaded.Defaults)
	}
}

func TestSettingsCycleModelAndEffort(t *testing.T) {
	s, _, _ := newSettingsForTest(t)
	s.field = sfModel
	before := s.draft.Defaults.Model
	s.cycle(+1)
	if s.draft.Defaults.Model == before {
		t.Errorf("model did not cycle from %q", before)
	}

	s.field = sfEffort
	beforeE := s.draft.Defaults.Effort
	s.cycle(+1)
	if s.draft.Defaults.Effort == beforeE {
		t.Errorf("effort did not cycle from %q", beforeE)
	}
}

func TestSettingsToggleWorktree(t *testing.T) {
	s, _, _ := newSettingsForTest(t)
	s.field = sfWorktree
	before := s.draft.Defaults.WorktreeOn
	s.cycle(+1)
	if s.draft.Defaults.WorktreeOn == before {
		t.Error("worktree toggle did not flip")
	}
}

func TestSettingsCyclePermission(t *testing.T) {
	s, _, _ := newSettingsForTest(t)
	s.field = sfPermission
	before := s.draft.Defaults.PermissionMode
	s.cycle(+1)
	if s.draft.Defaults.PermissionMode == before {
		t.Errorf("permission mode did not cycle from %q", before)
	}
	if !containsStr(permissionModes, s.draft.Defaults.PermissionMode) {
		t.Errorf("permission mode left the catalog: %q", s.draft.Defaults.PermissionMode)
	}
}

// Cycling the provider must leave model and effort valid for the new provider,
// even when the old effort was provider-specific (e.g. claude's "max").
func TestSettingsProviderCycleRevalidatesEffort(t *testing.T) {
	s, _, _ := newSettingsForTest(t)
	// Start on claude with an effort only claude supports.
	s.draft.Defaults.Provider = core.ProviderClaude
	s.draft.Defaults.Effort = core.EffortMax // claude-only
	s.field = sfProvider
	s.cycle(+1) // -> codex

	if s.draft.Defaults.Provider != core.ProviderCodex {
		t.Fatalf("provider should be codex, got %v", s.draft.Defaults.Provider)
	}
	if !core.SupportsEffort(core.ProviderCodex, s.draft.Defaults.Effort) {
		t.Errorf("effort %q is not valid for codex after the provider cycle", s.draft.Defaults.Effort)
	}
	if !containsStr(core.ProviderModels(core.ProviderCodex), s.draft.Defaults.Model) {
		t.Errorf("model %q is not valid for codex after the provider cycle", s.draft.Defaults.Model)
	}
}

func TestSettingsEscCloses(t *testing.T) {
	s, _, _ := newSettingsForTest(t)
	if !s.update(keyPress("esc")) {
		t.Error("esc should close settings")
	}
	if s.update(keyPress("j")) {
		t.Error("j should not close settings")
	}
}

func TestGoldenSettings(t *testing.T) {
	s, _, _ := newSettingsForTest(t)
	goldenFile(t, "settings", stripANSI(s.View()))
}
