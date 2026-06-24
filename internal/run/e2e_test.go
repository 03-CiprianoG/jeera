//go:build e2e

// This end-to-end test spawns a REAL claude agent and is excluded from normal
// builds/CI (which have no claude CLI). Run it manually:
//
//	go test -tags e2e -run TestE2E -v -timeout 5m ./internal/run/
package run

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/mcp"
	"github.com/03-CiprianoG/jeera/internal/store"
)

func TestE2EAgentRunsTicketOverMCP(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "jeera.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// A live MCP server over the same store.
	srv := mcp.NewServer(mcp.NewService(st))
	if err := srv.Start("127.0.0.1", 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })
	t.Logf("MCP at %s", srv.Status().URL)

	// A real git repo as the project.
	repo := gitRepo(t)
	p, _ := st.CreateProject(core.Project{Name: "E2E", KeyPrefix: "E2E", RepoPath: repo})
	iss, _ := st.CreateIssue(core.Issue{
		ProjectID:   p.ID,
		Title:       "Create a greeting file",
		Description: "Create a file named GREETING.md whose only content is the line:\n\nHello from Jeera\n\nThat is the entire task.",
		Assignee:    core.Assignee{Provider: core.ProviderClaude, Model: "haiku", Effort: core.EffortLow},
	})

	mgr := NewManager(st, t.TempDir(), func() string { return srv.Status().URL }, nil)
	r, err := mgr.Start(iss)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Logf("run %d started (session %s, worktree %s)", r.ID, r.SessionID, r.WorktreePath)

	// Poll until the run finishes.
	deadline := time.Now().Add(4 * time.Minute)
	var final core.Run
	for time.Now().Before(deadline) {
		final, _ = st.GetRun(r.ID)
		if final.Status.Terminal() {
			break
		}
		time.Sleep(time.Second)
	}
	if logs, err := os.ReadFile(final.LogPath); err == nil {
		t.Logf("--- run log (tail) ---\n%s", tail(string(logs), 2000))
	}
	if !final.Status.Terminal() {
		t.Fatalf("run did not finish; status=%s", final.Status)
	}
	t.Logf("run finished: status=%s exit=%v", final.Status, final.ExitCode)

	// The agent should have moved the ticket via MCP, and created the file in the
	// worktree.
	got, _ := st.GetIssue(iss.ID)
	stStatus, _ := st.GetStatus(got.StatusID)
	t.Logf("ticket status: %s (%s)", stStatus.Name, stStatus.Category)
	if stStatus.Category == core.CategoryTodo {
		t.Errorf("agent did not move the ticket out of To Do via MCP")
	}
	if _, err := os.Stat(filepath.Join(r.WorktreePath, "GREETING.md")); err != nil {
		t.Errorf("agent did not create GREETING.md in the worktree: %v", err)
	}
	comments, _ := st.ListComments(iss.ID)
	t.Logf("comments left by the agent: %d", len(comments))
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
