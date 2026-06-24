package version

import (
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	// Save and restore the package vars around the test.
	origV, origC, origD := Version, Commit, Date
	t.Cleanup(func() { Version, Commit, Date = origV, origC, origD })

	Version, Commit, Date = "1.2.3", "", ""
	if got := String(); got != "jeera 1.2.3" {
		t.Errorf("String() = %q, want %q", got, "jeera 1.2.3")
	}

	Version, Commit, Date = "1.2.3", "abc1234", "2026-06-24T00:00:00Z"
	got := String()
	for _, want := range []string{"jeera 1.2.3", "abc1234", "2026-06-24T00:00:00Z"} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, missing %q", got, want)
		}
	}
}

func TestShort(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })
	Version = "9.9.9"
	if Short() != "9.9.9" {
		t.Errorf("Short() = %q, want 9.9.9", Short())
	}
}
