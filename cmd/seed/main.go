// Command seed loads the Jeera store with a rich, self-consistent demo dataset
// so every capability — epics & subtasks, sprints & backlog, tags, links,
// comments, attachments, versioned runs, schedules and per-issue overrides —
// is visible in the TUI without any manual data entry.
//
// Usage:
//
//	go run ./cmd/seed            # seed the real store (paths.DBPath())
//	go run ./cmd/seed -db /tmp/demo.db
//	go run ./cmd/seed -reset     # delete a previous demo project first
//
// It only ever touches the demo project (key prefix "HEL"); other projects in
// the store are left untouched.
package main

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/paths"
	"github.com/03-CiprianoG/jeera/internal/store"
)

const demoPrefix = "HEL"

func main() {
	dbFlag := flag.String("db", "", "path to the SQLite store (default: the real Jeera DB)")
	reset := flag.Bool("reset", false, "delete an existing demo project before seeding")
	rich := flag.Bool("rich", false, "add only the flagship rich-content ticket to the existing demo project")
	flag.Parse()

	dbPath := *dbFlag
	if dbPath == "" {
		dbPath = paths.DBPath()
	}

	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open store %s: %v", dbPath, err)
	}
	defer st.Close()

	s := &seeder{st: st, now: time.Now()}

	// -rich is non-destructive: it (re)adds the showcase ticket to the existing
	// demo project without touching anything else.
	if *rich {
		proj, err := st.GetProjectByPrefix(demoPrefix)
		if err != nil {
			log.Fatalf("demo project %q not found — run the seeder without -rich first", demoPrefix)
		}
		iss := s.addRichTicket(proj)
		fmt.Printf("added flagship ticket %s — %q (%d attachments) to %s\n",
			iss.Key, iss.Title, s.flagshipAttachments, dbPath)
		return
	}

	if existing, err := st.GetProjectByPrefix(demoPrefix); err == nil {
		if !*reset {
			log.Fatalf("demo project %q already exists (id %d). Re-run with -reset to replace it.", demoPrefix, existing.ID)
		}
		if err := st.DeleteProject(existing.ID); err != nil {
			log.Fatalf("reset demo project: %v", err)
		}
		fmt.Printf("removed existing demo project (id %d)\n", existing.ID)
	}

	s.run(dbPath)
}

type seeder struct {
	st  *store.Store
	now time.Time

	// flagshipAttachments records how many attachments the rich ticket got, for
	// the -rich summary line.
	flagshipAttachments int
}

const flagshipTitle = "Helios v1.0 launch — architecture, rollout & on-call runbook"

// check fails fast — seeding is all-or-nothing for a demo.
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func ptr[T any](v T) *T { return &v }

func (s *seeder) days(n int) time.Time { return s.now.AddDate(0, 0, n) }

func (s *seeder) run(dbPath string) {
	repo := projectRepoPath()

	// --- project + board -----------------------------------------------------
	proj, err := s.st.CreateProject(core.Project{
		KeyPrefix: demoPrefix,
		Name:      "Helios — Realtime Analytics",
		RepoPath:  repo,
		Defaults: core.ProjectDefaults{
			Provider: core.ProviderClaude,
			Model:    "sonnet",
			Effort:   core.EffortMedium,
		},
	})
	check(err)

	// CreateProject seeds the default board: To Do / In Progress / In Review / Done.
	todo := s.status(proj.ID, "To Do")
	inProgress := s.status(proj.ID, "In Progress")
	inReview := s.status(proj.ID, "In Review")
	done := s.status(proj.ID, "Done")

	// --- sprints -------------------------------------------------------------
	sprint1, err := s.st.CreateSprint(core.Sprint{
		ProjectID: proj.ID, Name: "Sprint 1 · Foundations", State: core.SprintCompleted,
		Goal:    "Schemas, CI and a charting spike in place.",
		StartAt: ptr(s.days(-28)), EndAt: ptr(s.days(-14)),
	})
	check(err)
	sprint2, err := s.st.CreateSprint(core.Sprint{
		ProjectID: proj.ID, Name: "Sprint 2 · Ingestion MVP", State: core.SprintActive,
		Goal:    "Kafka consumer live and stable under load.",
		StartAt: ptr(s.days(-6)), EndAt: ptr(s.days(8)),
	})
	check(err)
	sprint3, err := s.st.CreateSprint(core.Sprint{
		ProjectID: proj.ID, Name: "Sprint 3 · Dashboards Alpha", State: core.SprintFuture,
		Goal:    "First customer-facing dashboards.",
		StartAt: ptr(s.days(9)), EndAt: ptr(s.days(23)),
	})
	check(err)

	// --- tags ----------------------------------------------------------------
	tag := func(name, color string) core.Tag {
		t, err := s.st.CreateTag(core.Tag{ProjectID: proj.ID, Name: name, Color: color})
		check(err)
		return t
	}
	backend := tag("backend", "#3B82F6")
	frontend := tag("frontend", "#A855F7")
	infra := tag("infra", "#F59E0B")
	bug := tag("bug", "#EF4444")
	techDebt := tag("tech-debt", "#6B7280")
	security := tag("security", "#10B981")

	// --- epics ---------------------------------------------------------------
	epicIngest := s.issue(issueSpec{
		proj: proj.ID, typ: core.TypeEpic, status: inProgress, prio: core.PriorityHigh,
		title:       "Realtime ingestion pipeline",
		description: "End-to-end streaming ingestion: Kafka consumer, schema registry, dead-letter queue and replay.",
		assignee:    asg(core.ProviderClaude, "opus", core.EffortHigh),
		tags:        []core.Tag{backend, infra},
	})
	epicDash := s.issue(issueSpec{
		proj: proj.ID, typ: core.TypeEpic, status: todo, prio: core.PriorityMedium,
		title:       "Customer-facing dashboards",
		description: "Layout engine, charts, saved views and sharing for end users.",
		assignee:    asg(core.ProviderClaude, "sonnet", core.EffortMedium),
		tags:        []core.Tag{frontend},
	})

	// --- ingestion epic children --------------------------------------------
	kafka := s.issue(issueSpec{
		proj: proj.ID, typ: core.TypeStory, status: inProgress, prio: core.PriorityHigh,
		title:       "Kafka consumer for event stream",
		description: "Consume the `events` topic, decode Avro, write to the columnar store.\n\n## Acceptance\n- at-least-once delivery\n- graceful rebalance\n- backpressure when the sink is slow",
		points:      ptr(8), epic: epicIngest.ID, sprint: sprint2.ID,
		assignee:   asg(core.ProviderClaude, "sonnet", core.EffortHigh),
		tags:       []core.Tag{backend, infra},
		worktreeOn: ptr(true),
	})
	schemas := s.issue(issueSpec{
		proj: proj.ID, typ: core.TypeTask, status: done, prio: core.PriorityMedium,
		title:       "Define Avro schemas for events",
		description: "Versioned Avro schemas registered in the schema registry.",
		points:      ptr(3), epic: epicIngest.ID, sprint: sprint1.ID,
		assignee: asg(core.ProviderClaude, "haiku", core.EffortMedium),
		tags:     []core.Tag{backend},
	})
	lag := s.issue(issueSpec{
		proj: proj.ID, typ: core.TypeBug, status: inReview, prio: core.PriorityHighest,
		title:       "Consumer lag spikes under load",
		description: "Under sustained 50k msg/s the consumer group lag grows unbounded. Suspect a blocking call in the decode path.",
		points:      ptr(5), epic: epicIngest.ID, sprint: sprint2.ID,
		assignee: asg(core.ProviderClaude, "opus", core.EffortMax),
		tags:     []core.Tag{backend, bug},
	})
	dlq := s.issue(issueSpec{
		proj: proj.ID, typ: core.TypeStory, status: todo, prio: core.PriorityMedium,
		title:       "Dead-letter queue + replay",
		description: "Route un-decodable messages to a DLQ topic and provide a replay command.",
		points:      ptr(5), epic: epicIngest.ID, // backlog: no sprint
		assignee: asg(core.ProviderCodex, "gpt-5.4", core.EffortMedium),
		tags:     []core.Tag{backend},
	})
	s.issue(issueSpec{
		proj: proj.ID, typ: core.TypeSubtask, status: todo, prio: core.PriorityMedium,
		title: "Provision DLQ topic + retention", parent: dlq.ID, points: ptr(1),
		assignee: asg(core.ProviderClaude, "haiku", core.EffortLow), tags: []core.Tag{infra},
	})
	s.issue(issueSpec{
		proj: proj.ID, typ: core.TypeSubtask, status: todo, prio: core.PriorityLow,
		title: "`jeera replay` CLI command", parent: dlq.ID, points: ptr(2),
		assignee: asg(core.ProviderCodex, "gpt-5.3-codex", core.EffortMedium), tags: []core.Tag{backend},
	})

	// --- dashboards epic children -------------------------------------------
	layout := s.issue(issueSpec{
		proj: proj.ID, typ: core.TypeStory, status: todo, prio: core.PriorityHigh,
		title:       "Dashboard layout engine",
		description: "Draggable grid with responsive breakpoints.",
		points:      ptr(13), epic: epicDash.ID, sprint: sprint3.ID,
		assignee: asg(core.ProviderClaude, "sonnet", core.EffortMedium),
		tags:     []core.Tag{frontend},
	})
	spike := s.issue(issueSpec{
		proj: proj.ID, typ: core.TypeTask, status: done, prio: core.PriorityLow,
		title:       "Charting library spike",
		description: "Evaluate charting libraries; recommendation written up and attached.",
		points:      ptr(2), epic: epicDash.ID, sprint: sprint1.ID,
		assignee: asg(core.ProviderClaude, "haiku", core.EffortLow),
		tags:     []core.Tag{frontend},
	})
	tooltip := s.issue(issueSpec{
		proj: proj.ID, typ: core.TypeBug, status: todo, prio: core.PriorityLow,
		title:       "Tooltip misaligns on Safari",
		description: "Chart tooltips render ~8px off on Safari 17.",
		points:      ptr(1), epic: epicDash.ID, // backlog
		assignee: asg(core.ProviderClaude, "haiku", core.EffortMedium),
		tags:     []core.Tag{frontend, bug},
	})
	s.issue(issueSpec{
		proj: proj.ID, typ: core.TypeStory, status: todo, prio: core.PriorityMedium,
		title:       "Saved views & sharing",
		description: "Persist dashboard configurations and share via link.",
		points:      ptr(8), epic: epicDash.ID, sprint: sprint3.ID,
		assignee: asg(core.ProviderCodex, "gpt-5.3-codex", core.EffortHigh),
		tags:     []core.Tag{frontend},
	})

	// --- epic-less platform work (infra / tech-debt / security) -------------
	ci := s.issue(issueSpec{
		proj: proj.ID, typ: core.TypeTask, status: done, prio: core.PriorityMedium,
		title: "CI: cache the Go build", points: ptr(2), sprint: sprint2.ID,
		assignee: asg(core.ProviderClaude, "haiku", core.EffortLow),
		tags:     []core.Tag{infra, techDebt},
	})
	s.issue(issueSpec{
		proj: proj.ID, typ: core.TypeTask, status: todo, prio: core.PriorityLow,
		title: "Upgrade toolchain to Go 1.24", points: ptr(3), // backlog, intentionally unassigned
		tags: []core.Tag{techDebt},
	})
	flaky := s.issue(issueSpec{
		proj: proj.ID, typ: core.TypeBug, status: inProgress, prio: core.PriorityHigh,
		title: "Flaky test in store package", points: ptr(2), sprint: sprint2.ID,
		description: "`TestRunVersioning` fails ~1/20 runs — looks like a timestamp race.",
		assignee:    asg(core.ProviderClaude, "sonnet", core.EffortHigh),
		tags:        []core.Tag{bug, techDebt},
	})
	audit := s.issue(issueSpec{
		proj: proj.ID, typ: core.TypeStory, status: todo, prio: core.PriorityHigh,
		title:       "Audit logging for MCP tools",
		description: "Record every MCP tool invocation with actor, args and result.",
		points:      ptr(5), // backlog
		assignee:    asg(core.ProviderClaude, "opus", core.EffortHigh),
		tags:        []core.Tag{security, backend},
		worktreeOn:  ptr(false),
		settings:    core.IssueSettings{PermissionMode: "bypassPermissions"},
	})

	// --- links (all four types) ---------------------------------------------
	s.link(proj.ID, schemas.ID, kafka.ID, core.LinkBlocks)     // schemas block the consumer
	s.link(proj.ID, dlq.ID, lag.ID, core.LinkBlockedBy)        // DLQ work waits on the lag fix
	s.link(proj.ID, lag.ID, kafka.ID, core.LinkRelates)        // related work
	s.link(proj.ID, tooltip.ID, flaky.ID, core.LinkDuplicates) // (illustrative) duplicate report

	// --- comments (human + agent) -------------------------------------------
	s.comment(kafka.ID, "human", "Let's target at-least-once first; exactly-once is out of scope for the MVP.")
	s.comment(lag.ID, "human", "Reproduced locally with the load generator at 50k msg/s.")

	// --- attachments (URL + file) -------------------------------------------
	s.attachURL(kafka.ID, "https://kafka.apache.org/documentation/#consumerconfigs")
	s.attachFile(spike.ID, s.writeDemoFile("charting-spike-recommendation.md",
		"# Charting spike\n\nRecommendation: use a lightweight Canvas renderer for time-series.\n"))

	// --- runs ----------------------------------------------------------------
	// Kafka story: a succeeded v1 and a running v2 forked from it (versioning +
	// lineage + an active run + a worktree).
	kafkaV1 := s.addRun(runSpec{
		issue: kafka.ID, provider: core.ProviderClaude, model: "sonnet", effort: core.EffortHigh,
		status: core.RunSucceeded, branch: "hel/kafka-consumer",
		worktree: filepath.Join(repo, ".worktrees", "HEL-kafka"),
		started:  ptr(s.days(-3)), ended: ptr(s.days(-3).Add(22 * time.Minute)), exit: ptr(0),
	})
	s.runComment(kafka.ID, kafkaV1.ID, "Implemented the consumer loop and rebalance handler. Tests green.")
	s.addRun(runSpec{
		issue: kafka.ID, provider: core.ProviderClaude, model: "sonnet", effort: core.EffortHigh,
		parent: ptr(kafkaV1.ID), status: core.RunRunning, branch: "hel/kafka-backpressure",
		worktree: filepath.Join(repo, ".worktrees", "HEL-kafka-2"),
		started:  ptr(s.now.Add(-12 * time.Minute)),
	})

	// Lag bug: a failed attempt then a successful fix.
	s.addRun(runSpec{
		issue: lag.ID, provider: core.ProviderClaude, model: "opus", effort: core.EffortMax,
		status: core.RunFailed, branch: "hel/lag-investigation",
		started: ptr(s.days(-1)), ended: ptr(s.days(-1).Add(9 * time.Minute)), exit: ptr(1),
	})
	s.addRun(runSpec{
		issue: lag.ID, provider: core.ProviderClaude, model: "opus", effort: core.EffortMax,
		status: core.RunSucceeded, branch: "hel/lag-fix",
		started: ptr(s.now.Add(-40 * time.Minute)), ended: ptr(s.now.Add(-7 * time.Minute)), exit: ptr(0),
	})

	s.addRun(runSpec{
		issue: ci.ID, provider: core.ProviderClaude, model: "haiku", effort: core.EffortLow,
		status: core.RunSucceeded, branch: "hel/ci-cache",
		started: ptr(s.days(-2)), ended: ptr(s.days(-2).Add(3 * time.Minute)), exit: ptr(0),
	})
	s.addRun(runSpec{ // currently executing
		issue: flaky.ID, provider: core.ProviderClaude, model: "sonnet", effort: core.EffortHigh,
		status: core.RunRunning, branch: "hel/flaky-store-test",
		started: ptr(s.now.Add(-4 * time.Minute)),
	})
	s.addRun(runSpec{ // queued, waiting for a slot
		issue: audit.ID, provider: core.ProviderClaude, model: "opus", effort: core.EffortHigh,
		status: core.RunQueued,
	})
	s.addRun(runSpec{ // operator cancelled
		issue: layout.ID, provider: core.ProviderClaude, model: "sonnet", effort: core.EffortMedium,
		status: core.RunCancelled, branch: "hel/layout-engine",
		started: ptr(s.days(-1)), ended: ptr(s.days(-1).Add(2 * time.Minute)),
	})
	s.addRun(runSpec{ // blocked by an unmet dependency
		issue: dlq.ID, provider: core.ProviderCodex, model: "gpt-5.4", effort: core.EffortMedium,
		status: core.RunBlocked,
	})

	// --- schedules -----------------------------------------------------------
	s.schedule(kafka.ID, "0 9 * * *", false, true, s.nextAt(9))                  // daily 09:00
	s.schedule(audit.ID, "0 6 * * 1", true, true, s.days(7).Truncate(time.Hour)) // weekly, with children

	// --- flagship ticket: a showcase of rich Markdown + mixed attachments ----
	s.addRichTicket(proj)

	// --- sprint backlog already handled via issue.sprint --------------------

	s.summary(proj, dbPath)
}

// ---- helpers ---------------------------------------------------------------

type issueSpec struct {
	proj        int64
	typ         core.IssueType
	title       string
	description string
	status      int64
	prio        core.Priority
	points      *int
	epic        int64
	parent      int64
	sprint      int64
	assignee    core.Assignee
	tags        []core.Tag
	worktreeOn  *bool
	settings    core.IssueSettings
}

func asg(p core.Provider, model string, e core.Effort) core.Assignee {
	return core.Assignee{Provider: p, Model: model, Effort: e}
}

func (s *seeder) status(projectID int64, name string) int64 {
	st, err := s.st.StatusByName(projectID, name)
	check(err)
	return st.ID
}

func (s *seeder) issue(spec issueSpec) core.Issue {
	iss := core.Issue{
		ProjectID:   spec.proj,
		Type:        spec.typ,
		Title:       spec.title,
		Description: spec.description,
		StatusID:    spec.status,
		Priority:    spec.prio,
		StoryPoints: spec.points,
		Assignee:    spec.assignee,
		WorktreeOn:  spec.worktreeOn,
		Settings:    spec.settings,
	}
	if spec.epic != 0 {
		iss.EpicID = ptr(spec.epic)
	}
	if spec.parent != 0 {
		iss.ParentID = ptr(spec.parent)
	}
	if spec.sprint != 0 {
		iss.SprintID = ptr(spec.sprint)
	}
	created, err := s.st.CreateIssue(iss)
	check(err)
	for _, t := range spec.tags {
		check(s.st.TagIssue(created.ID, t.ID))
	}
	return created
}

func (s *seeder) link(projectID, source, target int64, t core.LinkType) {
	_, err := s.st.CreateLink(core.IssueLink{ProjectID: projectID, SourceID: source, TargetID: target, Type: t})
	check(err)
}

func (s *seeder) comment(issueID int64, author, body string) {
	_, err := s.st.AddComment(core.Comment{IssueID: issueID, Author: author, Body: body})
	check(err)
}

func (s *seeder) runComment(issueID, runID int64, body string) {
	s.comment(issueID, fmt.Sprintf("run:%d", runID), body)
}

func (s *seeder) attachURL(issueID int64, ref string) {
	a := core.ClassifyAttachment(ref)
	a.IssueID = issueID
	_, err := s.st.CreateAttachment(a)
	check(err)
}

func (s *seeder) attachFile(issueID int64, path string) {
	a := core.ClassifyAttachment(path)
	a.IssueID = issueID
	if fi, err := os.Stat(path); err == nil {
		a.Size = fi.Size()
	}
	_, err := s.st.CreateAttachment(a)
	check(err)
}

// addRichTicket creates (or, idempotently, replaces) the flagship showcase
// ticket: a deep Markdown description plus a mix of URL links and real
// document/image file attachments, so the detail view's renderer is fully
// exercised. It is safe to call repeatedly.
func (s *seeder) addRichTicket(proj core.Project) core.Issue {
	// Idempotent: drop a previous flagship ticket before re-creating it.
	if existing, err := s.st.ListIssues(store.IssueFilter{ProjectID: proj.ID, Text: flagshipTitle}); err == nil {
		for _, iss := range existing {
			if iss.Title == flagshipTitle {
				check(s.st.DeleteIssue(iss.ID))
			}
		}
	}

	// Materialize real files so the attachments are genuinely openable.
	img := s.writeBinaryFile("helios-architecture.png", onePixelPNG())
	pdf := s.writeBinaryFile("helios-prd.pdf", minimalPDF("Helios v1.0 — Product Requirements"))
	runbook := s.writeDemoFile("rollout-runbook.md", runbookMarkdown)
	results := s.writeDemoFile("load-test-results.csv", loadTestCSV)

	// Prefer the active sprint and the In Progress column for a "live" feel.
	var sprintID int64
	if sprints, err := s.st.ListSprints(proj.ID); err == nil {
		for _, sp := range sprints {
			if sp.State == core.SprintActive {
				sprintID = sp.ID
				break
			}
		}
	}

	tagsByName := map[string]core.Tag{}
	if tags, err := s.st.ListTags(proj.ID); err == nil {
		for _, t := range tags {
			tagsByName[t.Name] = t
		}
	}
	var tags []core.Tag
	for _, name := range []string{"backend", "infra", "frontend", "security"} {
		if t, ok := tagsByName[name]; ok {
			tags = append(tags, t)
		}
	}

	desc := strings.NewReplacer("§", "`", "{{IMG}}", "file://"+img).Replace(flagshipDescription)

	iss := s.issue(issueSpec{
		proj: proj.ID, typ: core.TypeStory, status: s.status(proj.ID, "In Progress"),
		prio: core.PriorityHighest, title: flagshipTitle, description: desc,
		points: ptr(13), sprint: sprintID,
		assignee: asg(core.ProviderClaude, "opus", core.EffortHigh),
		tags:     tags, worktreeOn: ptr(true),
	})

	// Attachments: links first, then documents/images (7 total → the detail
	// view shows the first four and a "+3 more" line).
	s.attachURL(iss.ID, "https://github.com/03-CiprianoG/jeera")
	s.attachURL(iss.ID, "https://www.figma.com/file/HELIOS/dashboards-v1")
	s.attachURL(iss.ID, "https://grafana.helios.internal/d/ingest/overview")
	s.attachFile(iss.ID, img)
	s.attachFile(iss.ID, pdf)
	s.attachFile(iss.ID, runbook)
	s.attachFile(iss.ID, results)
	s.flagshipAttachments = 7

	// Activity: a human note and an agent run that posted a comment.
	run := s.addRun(runSpec{
		issue: iss.ID, provider: core.ProviderClaude, model: "opus", effort: core.EffortHigh,
		status: core.RunSucceeded, branch: "hel/v1-launch-prep",
		worktree: filepath.Join(proj.RepoPath, ".worktrees", "HEL-launch"),
		started:  ptr(s.now.Add(-55 * time.Minute)), ended: ptr(s.now.Add(-6 * time.Minute)), exit: ptr(0),
	})
	s.comment(iss.ID, "human", "PRD and runbook attached. Let's freeze scope after the load test signs off.")
	s.runComment(iss.ID, run.ID, "Drafted the rollout checklist and wired the canary feature flag. Load test results attached.")

	return iss
}

type runSpec struct {
	issue    int64
	provider core.Provider
	model    string
	effort   core.Effort
	status   core.RunStatus
	parent   *int64
	branch   string
	worktree string
	started  *time.Time
	ended    *time.Time
	exit     *int
}

func (s *seeder) addRun(spec runSpec) core.Run {
	r, err := s.st.CreateRun(core.Run{
		IssueID:      spec.issue,
		ParentRunID:  spec.parent,
		Provider:     spec.provider,
		Model:        spec.model,
		Effort:       spec.effort,
		Status:       spec.status,
		Branch:       spec.branch,
		WorktreePath: spec.worktree,
		SessionID:    fmt.Sprintf("sess-%d-%s", spec.issue, spec.status),
		StartedAt:    spec.started,
		EndedAt:      spec.ended,
		ExitCode:     spec.exit,
		LogPath:      filepath.Join(paths.DataDir(), "logs", fmt.Sprintf("run-%d.log", spec.issue)),
	})
	check(err)
	return r
}

func (s *seeder) schedule(issueID int64, cron string, withChildren, enabled bool, next time.Time) {
	_, err := s.st.CreateSchedule(core.Schedule{
		IssueID:      issueID,
		CronSpec:     cron,
		WithChildren: withChildren,
		Enabled:      enabled,
		NextRun:      ptr(next),
	})
	check(err)
}

// nextAt returns the next occurrence of the given hour, today or tomorrow.
func (s *seeder) nextAt(hour int) time.Time {
	t := time.Date(s.now.Year(), s.now.Month(), s.now.Day(), hour, 0, 0, 0, s.now.Location())
	if !t.After(s.now) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

func (s *seeder) writeDemoFile(name, content string) string {
	return s.writeBinaryFile(name, []byte(content))
}

func (s *seeder) writeBinaryFile(name string, content []byte) string {
	dir := filepath.Join(paths.DataDir(), "demo-attachments")
	check(os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, name)
	check(os.WriteFile(path, content, 0o644))
	return path
}

// onePixelPNG returns a valid 1×1 transparent PNG so the image attachment is a
// real, openable file rather than a placeholder.
func onePixelPNG() []byte {
	const b64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	data, err := base64.StdEncoding.DecodeString(b64)
	check(err)
	return data
}

// minimalPDF builds a small but structurally valid single-page PDF (correct
// xref offsets) that renders the given title, so the document attachment opens
// in any viewer.
func minimalPDF(title string) []byte {
	var b bytes.Buffer
	var offsets []int
	obj := func(s string) {
		offsets = append(offsets, b.Len())
		b.WriteString(s)
	}
	b.WriteString("%PDF-1.4\n")
	obj("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	obj("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	obj("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]" +
		" /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>\nendobj\n")
	obj("4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")
	content := fmt.Sprintf("BT /F1 24 Tf 72 700 Td (%s) Tj ET", title)
	obj(fmt.Sprintf("5 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(content), content))

	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	return b.Bytes()
}

// flagshipDescription is the rich Markdown body for the showcase ticket. § is a
// stand-in for a backtick (raw strings can't contain backticks) and {{IMG}} is
// replaced with the architecture image's file URL.
const flagshipDescription = `# Helios v1.0 launch

> **Goal:** ship realtime ingestion + the first customer dashboards to GA.
> Owner: _platform team_ · Target: **end of Sprint 2**.

Helios turns a firehose of product events into dashboards customers actually
read. This ticket is the **launch checklist** and the single source of truth for
the cutover. See the [repo](https://github.com/03-CiprianoG/jeera), the
[designs](https://www.figma.com/file/HELIOS/dashboards-v1) and live
[ingest metrics](https://grafana.helios.internal/d/ingest/overview).

## Architecture

![Helios architecture]({{IMG}})

The pipeline is three stages — **ingest → store → serve**:

| Stage  | Component        | Tech            | SLO            |
|--------|------------------|-----------------|----------------|
| Ingest | Kafka consumer   | Go              | < 2s p99 lag   |
| Store  | Columnar sink    | ClickHouse      | 99.9% writes   |
| Serve  | Dashboard API    | Go + React      | < 300ms p95    |

## Scope

- [x] Avro schemas registered
- [x] Kafka consumer (at-least-once)
- [ ] Dead-letter queue + replay
- [ ] Canary rollout behind a feature flag
- [ ] On-call runbook signed off

## Rollout plan

1. Deploy ingest to staging, replay 24h of events.
2. Enable the §helios_v1§ flag for **5%** of traffic (canary).
3. Watch p99 lag and error rate for 1h.
4. Ramp 5% → 50% → 100% if green; otherwise **roll back the flag**.

### Canary flag check

~~~go
// rollout gate evaluated per request
if flag.Enabled("helios_v1", tenant) {
    return serveV1(ctx, req)
}
return serveLegacy(ctx, req)
~~~

### One-line rollback

~~~bash
jeera flag set helios_v1 --percent 0   # instant kill-switch
~~~

## Risks

> ⚠️ Consumer lag spikes under load are **not yet fixed** — see the linked bug.
> Do **not** ramp past the canary until that closes.

---

**Attachments:** the PRD (PDF), the architecture diagram (PNG), the on-call
runbook (Markdown) and the latest load-test results (CSV) are pinned below.
~~~mermaid
graph LR; A[events] --> B[Kafka]; B --> C[ClickHouse]; C --> D[Dashboard API]
~~~`

const runbookMarkdown = `# Helios on-call runbook

## Alarms
- **IngestLagHigh** — consumer group lag > 2s for 5m.
- **WriteErrors** — ClickHouse write failures > 1%.

## First response
1. Check Grafana → "Ingest / Overview".
2. If lag only: scale the consumer deployment +2 replicas.
3. If write errors: flip ` + "`helios_v1`" + ` to 0% and page the DB owner.

## Escalation
platform-oncall → data-oncall → eng-lead.
`

const loadTestCSV = `scenario,rps,p50_ms,p95_ms,p99_ms,error_rate
baseline,10000,42,180,260,0.000
canary_5pct,12000,48,205,290,0.001
ramp_50pct,30000,61,240,330,0.002
peak_100pct,52000,79,295,410,0.004
`

func (s *seeder) summary(proj core.Project, dbPath string) {
	issues, err := s.st.ListIssues(store.IssueFilter{ProjectID: proj.ID})
	check(err)
	sprints, _ := s.st.ListSprints(proj.ID)
	tags, _ := s.st.ListTags(proj.ID)

	var runs int
	for _, iss := range issues {
		rs, _ := s.st.ListRuns(iss.ID)
		runs += len(rs)
	}

	fmt.Printf("\nseeded %q (%s) into %s\n", proj.Name, proj.KeyPrefix, dbPath)
	fmt.Printf("  issues:  %d (epics, stories, tasks, bugs, subtasks)\n", len(issues))
	fmt.Printf("  sprints: %d (active / future / completed)\n", len(sprints))
	fmt.Printf("  tags:    %d\n", len(tags))
	fmt.Printf("  runs:    %d across queued/running/succeeded/failed/cancelled/blocked\n", runs)
	fmt.Printf("\nRestart jeera to see the board.\n")
}

func projectRepoPath() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "/home/ubuntu/Jeera"
}
