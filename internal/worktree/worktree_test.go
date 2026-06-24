package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initRepo creates a tiny git repo with one commit for worktree tests.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "init")
	return dir
}

func TestWorktreeLifecycle(t *testing.T) {
	repo := initRepo(t)
	if !IsRepo(repo) {
		t.Fatal("IsRepo should be true for an initialized repo")
	}

	wtPath := filepath.Join(t.TempDir(), "wt-JEE-1")
	if err := Add(repo, wtPath, "jeera/JEE-1", "HEAD"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wtPath, "README.md")); err != nil {
		t.Errorf("worktree should contain the repo files: %v", err)
	}

	wts, err := List(repo)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var found bool
	for _, w := range wts {
		if w.Branch == "jeera/JEE-1" {
			found = true
		}
	}
	if !found {
		t.Errorf("new worktree branch not listed: %+v", wts)
	}

	if err := Remove(repo, wtPath, true); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree dir should be gone after Remove")
	}
}

func TestIsRepoFalse(t *testing.T) {
	if IsRepo(t.TempDir()) {
		t.Error("a plain temp dir is not a git repo")
	}
}

func TestSanitizeBranch(t *testing.T) {
	cases := map[string]string{
		"JEE-1":       "JEE-1",
		"feature foo": "feature-foo",
		"a:b?c*d":     "a-b-c-d",
		"  spaced  ":  "spaced",
	}
	for in, want := range cases {
		if got := SanitizeBranch(in); got != want {
			t.Errorf("SanitizeBranch(%q) = %q, want %q", in, got, want)
		}
	}
}
