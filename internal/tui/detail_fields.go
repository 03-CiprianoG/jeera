package tui

import "github.com/03-CiprianoG/jeera/internal/core"

func wrap(i, n int) int {
	if n == 0 {
		return 0
	}
	return ((i % n) + n) % n
}

// idxOf returns the index of v in s, or -1 if absent. Returning -1 (rather than
// 0) lets nextIdx distinguish "v is the first item" from "v isn't in the list"
// — important when a stored value (e.g. an out-of-catalog model) falls outside
// the cycle options.
func idxOf[T comparable](s []T, v T) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

// nextIdx advances cur by dir within n options. When cur is -1 (current value
// not in the list), it lands on the first option going forward and the last
// going backward, so a cycle from an unknown value is deterministic.
func nextIdx(cur, dir, n int) int {
	if n == 0 {
		return 0
	}
	if cur < 0 {
		if dir >= 0 {
			return 0
		}
		return n - 1
	}
	return wrap(cur+dir, n)
}

func (d *detailModel) statusIndex() int {
	for i, s := range d.statuses {
		if s.ID == d.issue.StatusID {
			return i
		}
	}
	return 0
}

func (d *detailModel) saveIssue() {
	if err := d.store.UpdateIssue(d.issue); err != nil {
		d.err = err.Error()
	}
}

// cycleField advances the currently-selected field's value by dir (+1/-1) and
// persists it. Points and Tags are edited via an input, so they no-op here.
func (d *detailModel) cycleField(dir int) {
	switch d.field {
	case dfStatus:
		if len(d.statuses) == 0 {
			return
		}
		next := wrap(d.statusIndex()+dir, len(d.statuses))
		if err := d.store.TransitionIssue(d.issue.ID, d.statuses[next].ID); err != nil {
			d.err = err.Error()
		}

	case dfType:
		types := core.IssueTypes()
		d.issue.Type = types[nextIdx(idxOf(types, d.issue.Type), dir, len(types))]
		d.saveIssue()

	case dfPriority:
		ps := core.Priorities()
		d.issue.Priority = ps[nextIdx(idxOf(ps, d.issue.Priority), dir, len(ps))]
		d.saveIssue()

	case dfProvider:
		// From unassigned, the first press selects the first provider rather
		// than skipping past it; thereafter it cycles.
		if d.issue.Assignee.IsZero() {
			d.issue.Assignee = core.DefaultAssignee(core.Providers()[0])
		} else {
			provs := core.Providers()
			d.issue.Assignee = core.DefaultAssignee(provs[nextIdx(idxOf(provs, d.issue.Assignee.Provider), dir, len(provs))])
		}
		d.saveIssue()

	case dfModel:
		if d.issue.Assignee.IsZero() {
			d.issue.Assignee = core.DefaultAssignee(core.ProviderClaude)
		} else if models := core.ProviderModels(d.issue.Assignee.Provider); len(models) > 0 {
			// nextIdx lands on the first model when the stored value is not in
			// this provider's catalog (e.g. a provider/model mismatch written
			// via the MCP set_assignee tool), instead of skipping it.
			d.issue.Assignee.Model = models[nextIdx(idxOf(models, d.issue.Assignee.Model), dir, len(models))]
		}
		d.saveIssue()

	case dfEffort:
		if d.issue.Assignee.IsZero() {
			d.issue.Assignee = core.DefaultAssignee(core.ProviderClaude)
		} else if efforts := core.ProviderEfforts(d.issue.Assignee.Provider); len(efforts) > 0 {
			d.issue.Assignee.Effort = efforts[nextIdx(idxOf(efforts, d.issue.Assignee.Effort), dir, len(efforts))]
		}
		d.saveIssue()

	case dfSprint:
		opts := append([]*int64{nil}, sprintIDs(d.sprints)...)
		sel := opts[wrap(indexInt64Ptr(opts, d.issue.SprintID)+dir, len(opts))]
		if err := d.store.AddIssueToSprint(d.issue.ID, sel); err != nil {
			d.err = err.Error()
		}

	case dfEpic:
		opts := append([]*int64{nil}, epicIDs(d.epics, d.issue.ID)...)
		d.issue.EpicID = opts[wrap(indexInt64Ptr(opts, d.issue.EpicID)+dir, len(opts))]
		d.saveIssue()
	}
	d.reload()
}

func sprintIDs(ss []core.Sprint) []*int64 {
	out := make([]*int64, len(ss))
	for i := range ss {
		id := ss[i].ID
		out[i] = &id
	}
	return out
}

// epicIDs returns the candidate epic IDs, excluding the issue itself so it can't
// be made its own parent.
func epicIDs(epics []core.Issue, self int64) []*int64 {
	out := make([]*int64, 0, len(epics))
	for i := range epics {
		if epics[i].ID == self {
			continue
		}
		id := epics[i].ID
		out = append(out, &id)
	}
	return out
}

func indexInt64Ptr(opts []*int64, cur *int64) int {
	for i, o := range opts {
		if (o == nil && cur == nil) || (o != nil && cur != nil && *o == *cur) {
			return i
		}
	}
	return 0
}
