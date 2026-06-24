package paths

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDataDir(t *testing.T) {
	t.Run("explicit override wins", func(t *testing.T) {
		t.Setenv("JEERA_DATA_DIR", "/custom/data")
		t.Setenv("XDG_DATA_HOME", "/xdg")
		if got := DataDir(); got != "/custom/data" {
			t.Errorf("DataDir() = %q, want /custom/data", got)
		}
	})
	t.Run("xdg fallback", func(t *testing.T) {
		t.Setenv("JEERA_DATA_DIR", "")
		t.Setenv("XDG_DATA_HOME", "/xdg")
		if got := DataDir(); got != filepath.Join("/xdg", "jeera") {
			t.Errorf("DataDir() = %q, want /xdg/jeera", got)
		}
	})
	t.Run("home default", func(t *testing.T) {
		t.Setenv("JEERA_DATA_DIR", "")
		t.Setenv("XDG_DATA_HOME", "")
		got := DataDir()
		if !strings.HasSuffix(got, filepath.Join(".local", "share", "jeera")) {
			t.Errorf("DataDir() = %q, want a ~/.local/share/jeera suffix", got)
		}
	})
}

func TestConfigDir(t *testing.T) {
	t.Run("explicit override wins", func(t *testing.T) {
		t.Setenv("JEERA_CONFIG_DIR", "/custom/cfg")
		t.Setenv("XDG_CONFIG_HOME", "/xdg")
		if got := ConfigDir(); got != "/custom/cfg" {
			t.Errorf("ConfigDir() = %q, want /custom/cfg", got)
		}
	})
	t.Run("xdg fallback", func(t *testing.T) {
		t.Setenv("JEERA_CONFIG_DIR", "")
		t.Setenv("XDG_CONFIG_HOME", "/xdg")
		if got := ConfigDir(); got != filepath.Join("/xdg", "jeera") {
			t.Errorf("ConfigDir() = %q, want /xdg/jeera", got)
		}
	})
}

func TestDerivedPaths(t *testing.T) {
	t.Setenv("JEERA_DATA_DIR", "/data")
	if got := DBPath(); got != filepath.Join("/data", "jeera.db") {
		t.Errorf("DBPath() = %q", got)
	}
	if got := LogsDir(); got != filepath.Join("/data", "logs") {
		t.Errorf("LogsDir() = %q", got)
	}
}
