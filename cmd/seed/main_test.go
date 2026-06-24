package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/store"
)

// seedTempStore runs the seeder against a throwaway store and returns it. It
// pins the data dir to the test's temp dir so the demo attachment/log paths
// never touch the real Jeera data directory.
func seedTempStore(t *testing.T) (*store.Store, core.Project) {
	t.Helper()
	t.Setenv("JEERA_DATA_DIR", t.TempDir())

	st, err := store.Open(filepath.Join(t.TempDir(), "jeera.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	s := &seeder{st: st, now: time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)}
	s.run(filepath.Join(t.TempDir(), "jeera.db"))

	proj, err := st.GetProjectByPrefix(demoPrefix)
	if err != nil {
		t.Fatalf("demo project not created: %v", err)
	}
	return st, proj
}

func TestSeedShape(t *testing.T) {
	st, proj := seedTempStore(t)

	issues, err := st.ListIssues(store.IssueFilter{ProjectID: proj.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 17 {
		t.Fatalf("issues = %d, want 17", len(issues))
	}

	// Every issue type is represented.
	types := map[core.IssueType]int{}
	for _, iss := range issues {
		types[iss.Type]++
	}
	for _, want := range core.IssueTypes() {
		if types[want] == 0 {
			t.Errorf("no issues of type %q", want)
		}
	}

	// Sprints cover all three states.
	sprints, err := st.ListSprints(proj.ID)
	if err != nil {
		t.Fatal(err)
	}
	states := map[core.SprintState]bool{}
	for _, sp := range sprints {
		states[sp.State] = true
	}
	for _, want := range []core.SprintState{core.SprintActive, core.SprintFuture, core.SprintCompleted} {
		if !states[want] {
			t.Errorf("no sprint in state %q", want)
		}
	}

	// At least one backlog (unsprinted) issue exists.
	backlog, err := st.ListIssues(store.IssueFilter{ProjectID: proj.ID, Unsprinted: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(backlog) == 0 {
		t.Error("expected at least one backlog issue")
	}
}

func TestSeedRunsCoverEveryStatus(t *testing.T) {
	st, proj := seedTempStore(t)

	issues, err := st.ListIssues(store.IssueFilter{ProjectID: proj.ID})
	if err != nil {
		t.Fatal(err)
	}

	seen := map[core.RunStatus]bool{}
	forked := false
	for _, iss := range issues {
		runs, err := st.ListRuns(iss.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range runs {
			seen[r.Status] = true
			if r.ParentRunID != nil {
				forked = true
			}
		}
	}

	for _, want := range []core.RunStatus{
		core.RunQueued, core.RunRunning, core.RunSucceeded,
		core.RunFailed, core.RunCancelled, core.RunBlocked,
	} {
		if !seen[want] {
			t.Errorf("no run with status %q", want)
		}
	}
	if !forked {
		t.Error("expected at least one forked run (ParentRunID set)")
	}
}

func TestSeedLinksAttachmentsAndSchedules(t *testing.T) {
	st, proj := seedTempStore(t)

	issues, err := st.ListIssues(store.IssueFilter{ProjectID: proj.ID})
	if err != nil {
		t.Fatal(err)
	}

	var url, file, schedules int
	linkTypes := map[core.LinkType]bool{}
	for _, iss := range issues {
		atts, err := st.ListAttachments(iss.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range atts {
			if a.IsURL() {
				url++
			} else {
				file++
			}
		}
		links, err := st.ListLinks(iss.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range links {
			linkTypes[l.Type] = true
		}
		scs, err := st.ListSchedules(iss.ID)
		if err != nil {
			t.Fatal(err)
		}
		schedules += len(scs)
	}

	if url == 0 || file == 0 {
		t.Errorf("attachments: url=%d file=%d, want at least one of each", url, file)
	}
	if schedules == 0 {
		t.Error("expected at least one schedule")
	}
	// ListLinks presents edges from either side, so the inverse of blocks
	// (blocked_by) also appears; assert the canonical types are all present.
	for _, want := range []core.LinkType{core.LinkBlocks, core.LinkRelates, core.LinkDuplicates} {
		if !linkTypes[want] {
			t.Errorf("no link of type %q", want)
		}
	}
}

func TestFlagshipTicket(t *testing.T) {
	st, proj := seedTempStore(t)

	issues, err := st.ListIssues(store.IssueFilter{ProjectID: proj.ID, Text: flagshipTitle})
	if err != nil {
		t.Fatal(err)
	}
	var flagship *core.Issue
	for i := range issues {
		if issues[i].Title == flagshipTitle {
			flagship = &issues[i]
		}
	}
	if flagship == nil {
		t.Fatal("flagship ticket not found")
	}

	// Rich Markdown body: headings, table, task list, fenced code, links, image.
	for _, frag := range []string{"# Helios v1.0 launch", "| Stage", "- [x]", "~~~go", "`helios_v1`", "https://github.com", "!["} {
		if !strings.Contains(flagship.Description, frag) {
			t.Errorf("description missing %q", frag)
		}
	}
	if strings.Contains(flagship.Description, "§") || strings.Contains(flagship.Description, "{{IMG}}") {
		t.Error("description still contains unreplaced placeholders")
	}

	// Mixed attachments: both URL links and real files, present on disk.
	atts, err := st.ListAttachments(flagship.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(atts) != 7 {
		t.Fatalf("flagship attachments = %d, want 7", len(atts))
	}
	var urls, files int
	for _, a := range atts {
		if a.IsURL() {
			urls++
			continue
		}
		files++
		if _, err := os.Stat(a.Path); err != nil {
			t.Errorf("attachment file %s not written: %v", a.Filename, err)
		}
	}
	if urls < 2 || files < 3 {
		t.Errorf("attachment mix urls=%d files=%d, want >=2 urls and >=3 files", urls, files)
	}
}

func TestRichTicketIsIdempotent(t *testing.T) {
	st, proj := seedTempStore(t)
	s := &seeder{st: st, now: time.Now()}

	// Re-adding the flagship ticket must not create a duplicate.
	s.addRichTicket(proj)
	s.addRichTicket(proj)

	issues, err := st.ListIssues(store.IssueFilter{ProjectID: proj.ID, Text: flagshipTitle})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	for _, iss := range issues {
		if iss.Title == flagshipTitle {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("found %d flagship tickets, want 1", n)
	}
}

func TestSeedResetIsIdempotent(t *testing.T) {
	t.Setenv("JEERA_DATA_DIR", t.TempDir())
	dbPath := filepath.Join(t.TempDir(), "jeera.db")

	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	(&seeder{st: st, now: time.Now()}).run(dbPath)

	// Simulate `-reset`: deleting and re-seeding must not duplicate the project.
	existing, err := st.GetProjectByPrefix(demoPrefix)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteProject(existing.ID); err != nil {
		t.Fatal(err)
	}
	(&seeder{st: st, now: time.Now()}).run(dbPath)

	projects, err := st.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	var hel int
	for _, p := range projects {
		if p.KeyPrefix == demoPrefix {
			hel++
		}
	}
	if hel != 1 {
		t.Fatalf("found %d %q projects after reset, want 1", hel, demoPrefix)
	}
}
