// Package schedule turns a persisted "Schedule Start" entry into a live cron job
// that runs a ticket while Jeera is up. Schedules live in the store, so they
// survive restarts; on boot the scheduler re-registers every enabled one. When a
// job fires it hands the issue to the run manager — the same path as pressing
// Start by hand — so a scheduled run is just an automated Start.
package schedule

import (
	"fmt"
	"strings"
	"sync"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/store"
)

// Runner starts a run for an issue. *run.Manager satisfies it; an interface keeps
// this package testable without spawning real agents.
type Runner interface {
	Start(issue core.Issue) (core.Run, error)
}

// Scheduler registers store-backed schedules as gocron jobs and fires them.
type Scheduler struct {
	store  *store.Store
	runner Runner
	cron   gocron.Scheduler

	mu   sync.Mutex
	jobs map[int64]liveJob // schedule id -> live gocron job
}

// liveJob ties a registered schedule to its gocron job and owning issue, so jobs
// can be torn down by issue (when the issue is deleted) as well as by schedule.
type liveJob struct {
	jobID   uuid.UUID
	issueID int64
}

// New builds a scheduler over the store and run manager. Call Start to load and
// begin firing the persisted schedules.
func New(st *store.Store, runner Runner) (*Scheduler, error) {
	c, err := gocron.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("schedule: new cron: %w", err)
	}
	return &Scheduler{store: st, runner: runner, cron: c, jobs: make(map[int64]liveJob)}, nil
}

// Start registers every enabled schedule and begins firing. Individual schedules
// with an invalid cron spec are skipped (and disabled) rather than aborting boot.
func (s *Scheduler) Start() error {
	schedules, err := s.store.ListEnabledSchedules()
	if err != nil {
		return err
	}
	for _, sc := range schedules {
		if _, err := s.register(sc); err != nil {
			// A spec that no longer parses shouldn't wedge startup; disable it so
			// the user can fix it.
			sc.Enabled = false
			_ = s.store.UpdateSchedule(sc)
		}
	}
	s.cron.Start()
	s.refreshNextRuns()
	return nil
}

// Add validates and persists a new schedule, registers it live, and returns it
// with its job binding and next-run time populated.
func (s *Scheduler) Add(issueID int64, cronSpec string, withChildren bool) (core.Schedule, error) {
	cronSpec = strings.TrimSpace(cronSpec)
	sc := core.Schedule{IssueID: issueID, CronSpec: cronSpec, WithChildren: withChildren, Enabled: true}
	sc, err := s.store.CreateSchedule(sc)
	if err != nil {
		return core.Schedule{}, err
	}
	sc, err = s.register(sc)
	if err != nil {
		// Roll back a schedule we can't actually run (e.g. a bad cron spec) so the
		// store never holds an unregistered, unfireable entry.
		_ = s.store.DeleteSchedule(sc.ID)
		return core.Schedule{}, fmt.Errorf("schedule: %w", err)
	}
	return sc, nil
}

// Remove unregisters a schedule's job and deletes it.
func (s *Scheduler) Remove(scheduleID int64) error {
	s.mu.Lock()
	lj, ok := s.jobs[scheduleID]
	delete(s.jobs, scheduleID)
	s.mu.Unlock()
	if ok {
		_ = s.cron.RemoveJob(lj.jobID)
	}
	return s.store.DeleteSchedule(scheduleID)
}

// RemoveForIssue stops every live job belonging to an issue. Call it when the
// issue is deleted: the schedule rows are removed by the issue's ON DELETE
// CASCADE, so this only has to drop the in-memory jobs that would otherwise keep
// firing (harmlessly, since fire() finds no issue) and leak in the jobs map.
func (s *Scheduler) RemoveForIssue(issueID int64) {
	s.mu.Lock()
	var stale []int64
	for schedID, lj := range s.jobs {
		if lj.issueID == issueID {
			_ = s.cron.RemoveJob(lj.jobID)
			stale = append(stale, schedID)
		}
	}
	for _, id := range stale {
		delete(s.jobs, id)
	}
	s.mu.Unlock()
}

// RemoveForProject stops every live job belonging to a project's issues. Call it
// just before the project is deleted: the schedule rows cascade away with the
// project, so this only has to drop the in-memory jobs that would otherwise keep
// firing (against now-deleted issues) and leak in the jobs map. It resolves each
// live job's issue to its project through the store, so it must run while the
// issues still exist — before the cascade, not after.
func (s *Scheduler) RemoveForProject(projectID int64) {
	// Snapshot the live jobs' issue IDs so the store lookups below don't run while
	// the lock is held (RemoveForIssue takes it again).
	s.mu.Lock()
	issueIDs := make(map[int64]struct{}, len(s.jobs))
	for _, lj := range s.jobs {
		issueIDs[lj.issueID] = struct{}{}
	}
	s.mu.Unlock()
	for issueID := range issueIDs {
		if iss, err := s.store.GetIssue(issueID); err == nil && iss.ProjectID == projectID {
			s.RemoveForIssue(issueID)
		}
	}
}

// ActiveJobs reports how many schedules are currently registered as live cron
// jobs — for status display and tests.
func (s *Scheduler) ActiveJobs() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.jobs)
}

// Shutdown stops the cron loop. Safe to call once.
func (s *Scheduler) Shutdown() error {
	if s.cron == nil {
		return nil
	}
	return s.cron.Shutdown()
}

// register binds a schedule to a gocron job, persisting the job id and next-run
// time. A 6-field spec is treated as having a leading seconds field.
func (s *Scheduler) register(sc core.Schedule) (core.Schedule, error) {
	withSeconds := len(strings.Fields(sc.CronSpec)) == 6
	job, err := s.cron.NewJob(
		gocron.CronJob(sc.CronSpec, withSeconds),
		gocron.NewTask(func() { s.fire(sc.IssueID) }),
	)
	if err != nil {
		return sc, err
	}
	s.mu.Lock()
	s.jobs[sc.ID] = liveJob{jobID: job.ID(), issueID: sc.IssueID}
	s.mu.Unlock()

	sc.JobUUID = job.ID().String()
	if next, err := job.NextRun(); err == nil && !next.IsZero() {
		n := next.UTC()
		sc.NextRun = &n
	}
	_ = s.store.UpdateSchedule(sc)
	return sc, nil
}

// fire is the job action: load the issue and hand it to the run manager. It is a
// method (not an inline closure) so its behavior is unit-testable directly.
func (s *Scheduler) fire(issueID int64) {
	iss, err := s.store.GetIssue(issueID)
	if err != nil {
		return
	}
	_, _ = s.runner.Start(iss)
}

// refreshNextRuns records the next-run time for each live job. NextRun is only
// available once the cron loop has started, so this runs after Start.
func (s *Scheduler) refreshNextRuns() {
	byID := make(map[uuid.UUID]gocron.Job)
	for _, j := range s.cron.Jobs() {
		byID[j.ID()] = j
	}
	s.mu.Lock()
	pairs := make(map[int64]gocron.Job, len(s.jobs))
	for schedID, lj := range s.jobs {
		if j, ok := byID[lj.jobID]; ok {
			pairs[schedID] = j
		}
	}
	s.mu.Unlock()

	for schedID, j := range pairs {
		next, err := j.NextRun()
		if err != nil || next.IsZero() {
			continue
		}
		sc, err := s.store.GetSchedule(schedID)
		if err != nil {
			continue
		}
		n := next.UTC()
		sc.NextRun = &n
		_ = s.store.UpdateSchedule(sc)
	}
}
