// Package worktree wraps the git worktree commands Jeera uses to isolate a
// ticket's run on its own branch, so an agent can work without disturbing the
// repository's checked-out state.
package worktree

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
)

// Worktree is one entry from `git worktree list`.
type Worktree struct {
	Path   string
	Branch string
	Head   string
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// Add creates a worktree at path. When branch is non-empty it is created with
// `-b branch`; base (a commit-ish) seeds it when given. repoDir must be inside a
// non-bare git repository.
func Add(repoDir, path, branch, base string) error {
	args := []string{"worktree", "add"}
	if branch != "" {
		args = append(args, "-b", branch)
	}
	args = append(args, path)
	if base != "" {
		args = append(args, base)
	}
	_, err := git(repoDir, args...)
	return err
}

// Remove deletes a worktree. A worktree with uncommitted changes requires force.
func Remove(repoDir, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	_, err := git(repoDir, args...)
	return err
}

// List returns the repository's worktrees, parsed from the porcelain format.
func List(repoDir string) ([]Worktree, error) {
	out, err := git(repoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var (
		result []Worktree
		cur    Worktree
	)
	flush := func() {
		if cur.Path != "" {
			result = append(result, cur)
		}
		cur = Worktree{}
	}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "worktree "):
			cur.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			cur.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		}
	}
	flush()
	return result, sc.Err()
}

// IsRepo reports whether dir is inside a git working tree.
func IsRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = dir
	return cmd.Run() == nil
}

// SanitizeBranch makes s safe to use as a git branch name component.
func SanitizeBranch(s string) string {
	repl := strings.NewReplacer(" ", "-", "~", "-", "^", "-", ":", "-", "?", "-", "*", "-", "[", "-", "\\", "-", "..", "-")
	return strings.Trim(repl.Replace(s), "-/")
}
