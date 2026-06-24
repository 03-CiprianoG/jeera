package run

import (
	"strings"
	"testing"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/store"
)

// makeChild creates a child issue of parent.
func makeChild(t *testing.T, st *store.Store, parent core.Issue, title string) core.Issue {
	t.Helper()
	pid := parent.ID
	c, err := st.CreateIssue(core.Issue{ProjectID: parent.ProjectID, Title: title, ParentID: &pid})
	if err != nil {
		t.Fatalf("CreateIssue(%s): %v", title, err)
	}
	return c
}

func TestChildrenThenSelfNoChildren(t *testing.T) {
	m, _, iss := setup(t)
	seq, err := m.childrenThenSelf(iss)
	if err != nil {
		t.Fatalf("childrenThenSelf: %v", err)
	}
	if len(seq) != 1 || seq[0].ID != iss.ID {
		t.Errorf("an issue with no children should sequence as just itself, got %d", len(seq))
	}
}

func TestChildrenThenSelfDependencyOrder(t *testing.T) {
	m, st, parent := setup(t)
	a := makeChild(t, st, parent, "A")
	b := makeChild(t, st, parent, "B")
	c := makeChild(t, st, parent, "C")
	// A blocks B, so A must run before B. (C is independent.)
	if _, err := st.CreateLink(core.IssueLink{SourceID: a.ID, TargetID: b.ID, Type: core.LinkBlocks}); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	seq, err := m.childrenThenSelf(parent)
	if err != nil {
		t.Fatalf("childrenThenSelf: %v", err)
	}
	// Parent must be last; all children present before it.
	if seq[len(seq)-1].ID != parent.ID {
		t.Errorf("parent should run last, got %d", seq[len(seq)-1].ID)
	}
	pos := map[int64]int{}
	for i, is := range seq {
		pos[is.ID] = i
	}
	if pos[a.ID] > pos[b.ID] {
		t.Errorf("blocker A (%d) should precede blocked B: order a=%d b=%d", a.ID, pos[a.ID], pos[b.ID])
	}
	if _, ok := pos[c.ID]; !ok {
		t.Error("independent child C should still be in the sequence")
	}
	if len(seq) != 4 {
		t.Errorf("expected 3 children + parent = 4, got %d", len(seq))
	}
}

// A transitive chain A→B→C must sort correctly even when the input is unordered,
// proving the topological propagation rather than coincidental seed order.
func TestDependencyOrderTransitiveChain(t *testing.T) {
	m, st, parent := setup(t)
	a := makeChild(t, st, parent, "A")
	b := makeChild(t, st, parent, "B")
	c := makeChild(t, st, parent, "C")
	// A blocks B, B blocks C.
	st.CreateLink(core.IssueLink{SourceID: a.ID, TargetID: b.ID, Type: core.LinkBlocks})
	st.CreateLink(core.IssueLink{SourceID: b.ID, TargetID: c.ID, Type: core.LinkBlocks})

	// Feed the children out of dependency order so seed order can't accidentally
	// produce the right answer.
	ordered := m.dependencyOrder([]core.Issue{c, a, b})
	pos := map[int64]int{}
	for i, is := range ordered {
		pos[is.ID] = i
	}
	if !(pos[a.ID] < pos[b.ID] && pos[b.ID] < pos[c.ID]) {
		t.Errorf("chain should sort A<B<C, got a=%d b=%d c=%d", pos[a.ID], pos[b.ID], pos[c.ID])
	}
}

// A dependency cycle must not hang or drop issues — they degrade to input order.
func TestDependencyOrderCycleDegrades(t *testing.T) {
	m, st, parent := setup(t)
	a := makeChild(t, st, parent, "A")
	b := makeChild(t, st, parent, "B")
	// A blocks B and B blocks A — a cycle.
	st.CreateLink(core.IssueLink{SourceID: a.ID, TargetID: b.ID, Type: core.LinkBlocks})
	st.CreateLink(core.IssueLink{SourceID: b.ID, TargetID: a.ID, Type: core.LinkBlocks})

	ordered := m.dependencyOrder([]core.Issue{a, b})
	if len(ordered) != 2 {
		t.Errorf("a cycle should keep all issues, got %d", len(ordered))
	}
}

// After Shutdown, a sequenced StartWithChildren must not begin new runs.
func TestStartWithChildrenAbortsOnShutdown(t *testing.T) {
	m, st, parent := setup(t)
	makeChild(t, st, parent, "A")
	makeChild(t, st, parent, "B")

	m.Shutdown() // cancel the lifecycle before sequencing

	if err := m.StartWithChildren(parent); err != nil {
		t.Fatalf("StartWithChildren: %v", err)
	}
	m.wg.Wait() // the sequence goroutine should return immediately

	// No runs should have been created, since the manager was already shut down.
	for _, iss := range mustChildrenAndParent(t, st, parent) {
		runs, _ := st.ListRuns(iss.ID)
		if len(runs) != 0 {
			t.Errorf("issue %d should have no runs after shutdown, got %d", iss.ID, len(runs))
		}
	}
}

func mustChildrenAndParent(t *testing.T, st *store.Store, parent core.Issue) []core.Issue {
	t.Helper()
	kids, err := st.ListIssues(store.IssueFilter{ParentID: &parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	return append(kids, parent)
}

func TestDiscussCommandArgs(t *testing.T) {
	m, _, iss := setup(t) // setup's mcpURL returns a non-empty URL
	cmd, err := m.DiscussCommand(iss)
	if err != nil {
		t.Fatalf("DiscussCommand: %v", err)
	}
	if cmd.Args[0] != "claude" {
		t.Errorf("discuss should use claude, got %q", cmd.Args[0])
	}
	joined := strings.Join(cmd.Args, " ")
	for _, want := range []string{"--mcp-config", "--strict-mcp-config", iss.Key} {
		if !strings.Contains(joined, want) {
			t.Errorf("discuss args missing %q in:\n%s", want, joined)
		}
	}
	// It must NOT be a -p (headless) run — Discuss is interactive.
	if strings.Contains(joined, "-p ") {
		t.Errorf("discuss should be interactive, not -p: %s", joined)
	}
	project, _ := m.store.GetProject(iss.ProjectID)
	if cmd.Dir != project.RepoPath {
		t.Errorf("discuss should run in the repo, dir=%q want %q", cmd.Dir, project.RepoPath)
	}
}

func TestDiscussCommandNoMCP(t *testing.T) {
	_, st, iss := setup(t)
	// A manager with no live MCP endpoint cannot discuss — the agent would have
	// nothing to load the ticket from.
	m := NewManager(st, t.TempDir(), func() string { return "" }, nil)
	if _, err := m.DiscussCommand(iss); err == nil {
		t.Error("DiscussCommand should error when the MCP server is off")
	}
}
