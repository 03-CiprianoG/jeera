// Package config is Jeera's settings cascade. A single global TOML file holds the
// fallback run settings; a project can override them with its own defaults; and an
// individual issue overrides those. ResolveRun collapses the three layers into the
// concrete settings a run uses, so "Start" behaves predictably whether a ticket
// was filled in fully, partially, or not at all.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/BurntSushi/toml"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/paths"
)

// Defaults are the fallback run settings. They live at the global (config file)
// layer and, partially, at the project layer via core.ProjectDefaults.
type Defaults struct {
	Provider       core.Provider `toml:"provider"`
	Model          string        `toml:"model"`
	Effort         core.Effort   `toml:"effort"`
	WorktreeOn     bool          `toml:"worktree_on"`
	PermissionMode string        `toml:"permission_mode"`
}

// Config is the whole global configuration file.
type Config struct {
	Defaults Defaults `toml:"defaults"`
	// MCPPort is the preferred MCP server port (0 = use the built-in default and
	// fall back to a free port if it is taken).
	MCPPort int `toml:"mcp_port"`
}

// Default returns the built-in configuration used when no file exists yet: the
// catalog's default Claude assignee, worktrees on, fully-autonomous permissions.
func Default() Config {
	a := core.DefaultAssignee(core.ProviderClaude)
	return Config{
		Defaults: Defaults{
			Provider:       a.Provider,
			Model:          a.Model,
			Effort:         a.Effort,
			WorktreeOn:     true,
			PermissionMode: "bypassPermissions",
		},
		MCPPort: 0,
	}
}

// Path is the default config file location (under the XDG config dir).
func Path() string { return filepath.Join(paths.ConfigDir(), "config.toml") }

// Load reads the config at path on top of Default(), so any field the file omits
// keeps its built-in value. A missing file is not an error — it yields Default().
func Load(path string) (Config, error) {
	cfg := Default()
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, fmt.Errorf("config: read %s: %w", path, err)
	}
	return cfg, nil
}

// Save writes cfg to path, creating the directory and replacing the file
// atomically so a crash mid-write can't leave a truncated config.
func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("config: mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.toml")
	if err != nil {
		return fmt.Errorf("config: temp: %w", err)
	}
	defer os.Remove(tmp.Name()) // no-op once the rename succeeds
	if err := toml.NewEncoder(tmp).Encode(cfg); err != nil {
		tmp.Close()
		return fmt.Errorf("config: encode: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("config: close: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("config: replace: %w", err)
	}
	return nil
}

// Store is a live, mutable view of the global configuration. It is safe for
// concurrent use: the settings UI writes it on the main loop while the scheduler
// reads the defaults from its own goroutine when a timed run fires. Save persists
// to disk and updates the in-memory copy together.
type Store struct {
	mu   sync.RWMutex
	cfg  Config
	path string
}

// NewStore loads the config at path into a live Store. It always returns a usable
// Store (with Default() values on a read error), plus any load error to surface.
func NewStore(path string) (*Store, error) {
	cfg, err := Load(path)
	return &Store{cfg: cfg, path: path}, err
}

// Get returns a copy of the current configuration.
func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Defaults returns the current global defaults — a func value suitable as the run
// manager's live defaults getter.
func (s *Store) Defaults() Defaults { return s.Get().Defaults }

// Save persists cfg and, on success, swaps it in as the live configuration.
func (s *Store) Save(cfg Config) error {
	if err := Save(s.path, cfg); err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	return nil
}

// RunSettings is the concrete result of resolving the cascade for one run.
type RunSettings struct {
	Assignee       core.Assignee
	WorktreeOn     bool
	PermissionMode string
}

// ResolveRun collapses the issue → project → global cascade into the settings a
// run will use. Each field is taken from the most specific layer that sets it:
// the issue's own value, then the project default, then the global default. The
// assignee is resolved field by field, and a provider with no usable model falls
// back to that provider's catalog default so a run is never launched with an
// empty model.
func ResolveRun(global Defaults, project core.Project, issue core.Issue) RunSettings {
	pd := project.Defaults

	// Assignee: the issue's is all-or-nothing (set in the detail view); otherwise
	// coalesce provider/model/effort across the project and global layers.
	assignee := issue.Assignee
	if assignee.IsZero() {
		assignee = core.Assignee{
			Provider: firstProvider(pd.Provider, global.Provider, core.ProviderClaude),
			Model:    firstNonEmpty(pd.Model, global.Model),
			Effort:   firstEffort(pd.Effort, global.Effort, core.EffortMedium),
		}
	}
	// Guard the resolved assignee — whichever layer produced it — so a run never
	// launches with a model or effort the provider doesn't support. A mismatch can
	// reach here from a project forcing a different provider than the global model,
	// or from a pair persisted on the issue via the MCP set_assignee tool / a
	// hand-edited config. Fall back to the provider's catalog defaults.
	if !validModel(assignee.Provider, assignee.Model) {
		assignee.Model = core.DefaultAssignee(assignee.Provider).Model
	}
	if !core.SupportsEffort(assignee.Provider, assignee.Effort) {
		assignee.Effort = core.DefaultAssignee(assignee.Provider).Effort
	}

	// Worktree: issue pointer → project pointer → global bool.
	worktree := global.WorktreeOn
	if pd.WorktreeOn != nil {
		worktree = *pd.WorktreeOn
	}
	if issue.WorktreeOn != nil {
		worktree = *issue.WorktreeOn
	}

	// Permission mode: issue → project → global, with a safe final fallback.
	perm := firstNonEmpty(issue.Settings.PermissionMode, pd.PermissionMode, global.PermissionMode)
	if perm == "" {
		perm = "bypassPermissions"
	}

	return RunSettings{Assignee: assignee, WorktreeOn: worktree, PermissionMode: perm}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstProvider(vals ...core.Provider) core.Provider {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstEffort(vals ...core.Effort) core.Effort {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func validModel(p core.Provider, model string) bool {
	if model == "" {
		return false
	}
	for _, m := range core.ProviderModels(p) {
		if m == model {
			return true
		}
	}
	return false
}
