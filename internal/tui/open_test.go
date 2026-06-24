package tui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOpenCommandIncludesRef(t *testing.T) {
	cmd := openCommand("https://example.com/spec")
	if !strings.Contains(strings.Join(cmd.Args, " "), "https://example.com/spec") {
		t.Errorf("open command should include the ref: %v", cmd.Args)
	}
}

func TestOpenCommandBinaryPerOS(t *testing.T) {
	cmd := openCommand("/x/y.png")
	want := map[string]string{"darwin": "open", "windows": "rundll32"}[runtime.GOOS]
	if want == "" {
		want = "xdg-open"
	}
	if cmd.Args[0] != want {
		t.Errorf("opener on %s = %q, want %q", runtime.GOOS, cmd.Args[0], want)
	}
}

func TestSafeToOpenAllowsHTTPS(t *testing.T) {
	if err := safeToOpen("https://example.com/a"); err != nil {
		t.Errorf("https URL should be openable: %v", err)
	}
	if err := safeToOpen("http://example.com"); err != nil {
		t.Errorf("http URL should be openable: %v", err)
	}
}

func TestSafeToOpenAllowsExistingFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "doc.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := safeToOpen(f); err != nil {
		t.Errorf("an existing regular file should be openable: %v", err)
	}
}

func TestSafeToOpenRejectsDangerous(t *testing.T) {
	cases := []string{
		"file:///etc/passwd",
		"javascript:alert(1)",
		"smb://attacker/share",
		"vbscript:Execute",
		`\\attacker\share\x`,
		"/does/not/exist/anywhere.png",
		"", // empty
	}
	for _, ref := range cases {
		if err := safeToOpen(ref); err == nil {
			t.Errorf("safeToOpen(%q) should be refused", ref)
		}
	}
}

func TestSafeToOpenRejectsDirectory(t *testing.T) {
	if err := safeToOpen(t.TempDir()); err == nil {
		t.Error("a directory should not be openable")
	}
}
