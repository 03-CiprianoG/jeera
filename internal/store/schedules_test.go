package store

import (
	"errors"
	"testing"
	"time"

	"github.com/03-CiprianoG/jeera/internal/core"
)

func TestScheduleLifecycle(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)
	iss := mustIssue(t, s, p.ID, "nightly")

	next := time.Now().UTC().Add(time.Hour)
	sc, err := s.CreateSchedule(core.Schedule{
		IssueID: iss.ID, CronSpec: "0 9 * * *", WithChildren: true, Enabled: true,
		JobUUID: "job-1", NextRun: &next,
	})
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}
	if sc.ID == 0 {
		t.Fatalf("schedule should have an id: %+v", sc)
	}

	got, err := s.GetSchedule(sc.ID)
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}
	if got.CronSpec != "0 9 * * *" || !got.WithChildren || !got.Enabled || got.JobUUID != "job-1" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.NextRun == nil || !got.NextRun.Equal(next) {
		t.Errorf("next run not persisted: %v want %v", got.NextRun, next)
	}

	// Update: disable, rebind the job, clear the next run.
	got.Enabled = false
	got.JobUUID = "job-2"
	got.NextRun = nil
	if err := s.UpdateSchedule(got); err != nil {
		t.Fatalf("UpdateSchedule: %v", err)
	}
	reloaded, _ := s.GetSchedule(sc.ID)
	if reloaded.Enabled || reloaded.JobUUID != "job-2" || reloaded.NextRun != nil {
		t.Errorf("update not persisted: %+v", reloaded)
	}
}

func TestListEnabledSchedules(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)
	a := mustIssue(t, s, p.ID, "a")
	b := mustIssue(t, s, p.ID, "b")

	s.CreateSchedule(core.Schedule{IssueID: a.ID, CronSpec: "* * * * *", Enabled: true})
	s.CreateSchedule(core.Schedule{IssueID: b.ID, CronSpec: "* * * * *", Enabled: false})

	enabled, err := s.ListEnabledSchedules()
	if err != nil {
		t.Fatalf("ListEnabledSchedules: %v", err)
	}
	if len(enabled) != 1 || enabled[0].IssueID != a.ID {
		t.Errorf("expected only the enabled schedule, got %+v", enabled)
	}

	// Per-issue listing.
	forA, _ := s.ListSchedules(a.ID)
	if len(forA) != 1 {
		t.Errorf("ListSchedules(a) = %d, want 1", len(forA))
	}
}

func TestDeleteSchedule(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)
	iss := mustIssue(t, s, p.ID, "doomed")
	sc, _ := s.CreateSchedule(core.Schedule{IssueID: iss.ID, CronSpec: "* * * * *", Enabled: true})

	if err := s.DeleteSchedule(sc.ID); err != nil {
		t.Fatalf("DeleteSchedule: %v", err)
	}
	if _, err := s.GetSchedule(sc.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("schedule should be gone, got err=%v", err)
	}
	if err := s.DeleteSchedule(sc.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting a missing schedule should be ErrNotFound, got %v", err)
	}
}

// Deleting an issue cascades to its schedules.
func TestScheduleCascadesWithIssue(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s)
	iss := mustIssue(t, s, p.ID, "tied")
	s.CreateSchedule(core.Schedule{IssueID: iss.ID, CronSpec: "* * * * *", Enabled: true})

	if err := s.DeleteIssue(iss.ID); err != nil {
		t.Fatalf("DeleteIssue: %v", err)
	}
	left, _ := s.ListSchedules(iss.ID)
	if len(left) != 0 {
		t.Errorf("schedules should cascade-delete with the issue, got %d", len(left))
	}
}
