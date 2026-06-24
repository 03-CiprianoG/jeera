package schedule

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/store"
)

// fakeRunner records the issues it was asked to start.
type fakeRunner struct {
	mu      sync.Mutex
	started []int64
}

func (f *fakeRunner) Start(iss core.Issue) (core.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = append(f.started, iss.ID)
	return core.Run{IssueID: iss.ID}, nil
}

func (f *fakeRunner) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.started)
}

func setup(t *testing.T) (*store.Store, core.Issue, *fakeRunner) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "jeera.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, _ := st.CreateProject(core.Project{Name: "Jeera", KeyPrefix: "JEE", RepoPath: "/tmp/x"})
	iss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "scheduled"})
	return st, iss, &fakeRunner{}
}

func newScheduler(t *testing.T, st *store.Store, r Runner) *Scheduler {
	t.Helper()
	s, err := New(st, r)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Shutdown() })
	return s
}

func TestAddPersistsAndBindsJob(t *testing.T) {
	st, iss, r := setup(t)
	s := newScheduler(t, st, r)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	sc, err := s.Add(iss.ID, "0 9 * * *", true)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if sc.JobUUID == "" {
		t.Error("schedule should be bound to a job uuid")
	}
	if sc.NextRun == nil || !sc.NextRun.After(time.Now()) {
		t.Errorf("next run should be in the future, got %v", sc.NextRun)
	}
	// Persisted and reflected in the store.
	got, _ := st.GetSchedule(sc.ID)
	if got.JobUUID != sc.JobUUID || got.NextRun == nil {
		t.Errorf("schedule not persisted with job binding: %+v", got)
	}
}

func TestAddRejectsBadCron(t *testing.T) {
	st, iss, r := setup(t)
	s := newScheduler(t, st, r)
	_ = s.Start()

	if _, err := s.Add(iss.ID, "not a cron spec", false); err == nil {
		t.Fatal("expected an error for an invalid cron spec")
	}
	// The rolled-back schedule must not linger in the store.
	all, _ := st.ListSchedules(iss.ID)
	if len(all) != 0 {
		t.Errorf("a rejected schedule should be rolled back, found %d", len(all))
	}
}

func TestRemoveUnregistersAndDeletes(t *testing.T) {
	st, iss, r := setup(t)
	s := newScheduler(t, st, r)
	_ = s.Start()
	sc, _ := s.Add(iss.ID, "0 9 * * *", false)

	if err := s.Remove(sc.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if all, _ := st.ListSchedules(iss.ID); len(all) != 0 {
		t.Errorf("schedule should be deleted, found %d", len(all))
	}
	if len(s.cron.Jobs()) != 0 {
		t.Errorf("gocron job should be removed, %d remain", len(s.cron.Jobs()))
	}
}

// On boot, only enabled schedules are re-registered.
func TestStartReRegistersEnabledOnly(t *testing.T) {
	st, iss, r := setup(t)
	st.CreateSchedule(core.Schedule{IssueID: iss.ID, CronSpec: "0 9 * * *", Enabled: true})
	st.CreateSchedule(core.Schedule{IssueID: iss.ID, CronSpec: "0 10 * * *", Enabled: false})

	s := newScheduler(t, st, r)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(s.cron.Jobs()) != 1 {
		t.Errorf("only the enabled schedule should be registered, got %d jobs", len(s.cron.Jobs()))
	}
}

// A bad spec already in the store is disabled, not fatal.
func TestStartDisablesUnparseableSchedule(t *testing.T) {
	st, iss, r := setup(t)
	bad, _ := st.CreateSchedule(core.Schedule{IssueID: iss.ID, CronSpec: "garbage", Enabled: true})

	s := newScheduler(t, st, r)
	if err := s.Start(); err != nil {
		t.Fatalf("Start should not fail on a bad schedule: %v", err)
	}
	got, _ := st.GetSchedule(bad.ID)
	if got.Enabled {
		t.Error("an unparseable schedule should be disabled on boot")
	}
}

// Deleting an issue must stop its live jobs, not just its store rows.
func TestRemoveForIssueStopsJobs(t *testing.T) {
	st, iss, r := setup(t)
	other, _ := st.CreateIssue(core.Issue{ProjectID: iss.ProjectID, Title: "survivor"})
	s := newScheduler(t, st, r)
	_ = s.Start()
	s.Add(iss.ID, "0 9 * * *", false)
	s.Add(iss.ID, "0 18 * * *", false) // a second schedule on the same issue
	s.Add(other.ID, "0 12 * * *", false)

	s.RemoveForIssue(iss.ID)

	if len(s.cron.Jobs()) != 1 {
		t.Errorf("only the other issue's job should remain, got %d", len(s.cron.Jobs()))
	}
	s.mu.Lock()
	left := len(s.jobs)
	s.mu.Unlock()
	if left != 1 {
		t.Errorf("jobs map should hold only the survivor, got %d", left)
	}
}

func TestFireStartsTheIssue(t *testing.T) {
	st, iss, r := setup(t)
	s := newScheduler(t, st, r)
	s.fire(iss.ID)
	if r.count() != 1 || r.started[0] != iss.ID {
		t.Errorf("fire should start the issue once, got %v", r.started)
	}
	// A missing issue is a no-op, not a crash.
	s.fire(999999)
	if r.count() != 1 {
		t.Errorf("firing a missing issue should be a no-op, got %v", r.started)
	}
}

// End-to-end: an every-second schedule actually fires the run manager.
func TestEverySecondScheduleFires(t *testing.T) {
	st, iss, r := setup(t)
	s := newScheduler(t, st, r)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := s.Add(iss.ID, "* * * * * *", false); err != nil { // 6-field: every second
		t.Fatalf("Add: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if r.count() > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("an every-second schedule never fired")
}
