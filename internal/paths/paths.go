// Package paths resolves the on-disk locations Jeera uses, following the XDG
// Base Directory conventions so the single binary behaves predictably across
// machines. Every location can be overridden with an environment variable to
// keep development and tests isolated from a real install.
package paths

import (
	"os"
	"path/filepath"
)

// DataDir is where Jeera keeps its system-of-record database and run logs.
// Resolution order: JEERA_DATA_DIR, $XDG_DATA_HOME/jeera, ~/.local/share/jeera.
func DataDir() string {
	if d := os.Getenv("JEERA_DATA_DIR"); d != "" {
		return d
	}
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "jeera")
	}
	return filepath.Join(home(), ".local", "share", "jeera")
}

// ConfigDir is where Jeera keeps its TOML configuration.
// Resolution order: JEERA_CONFIG_DIR, $XDG_CONFIG_HOME/jeera, ~/.config/jeera.
func ConfigDir() string {
	if d := os.Getenv("JEERA_CONFIG_DIR"); d != "" {
		return d
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "jeera")
	}
	return filepath.Join(home(), ".config", "jeera")
}

// DBPath is the SQLite database file inside DataDir.
func DBPath() string { return filepath.Join(DataDir(), "jeera.db") }

// LogsDir is where per-run execution logs are written.
func LogsDir() string { return filepath.Join(DataDir(), "logs") }

// home returns the user's home directory, falling back to "." if it cannot be
// determined so callers still get a usable relative path rather than an empty
// one.
func home() string {
	h, err := os.UserHomeDir()
	if err != nil || h == "" {
		return "."
	}
	return h
}
