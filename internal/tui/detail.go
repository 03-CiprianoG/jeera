package tui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/run"
	"github.com/03-CiprianoG/jeera/internal/schedule"
	"github.com/03-CiprianoG/jeera/internal/store"
	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

// detailField enumerates the editable metadata fields in the sidebar, navigated
// with j/k and cycled with h/l.
type detailField int

const (
	dfStatus detailField = iota
	dfType
	dfPriority
	dfPoints
	dfProvider
	dfModel
	dfEffort
	dfSprint
	dfEpic
	dfTags
	dfFieldCount
)

type detailMode int

const (
	dViewing detailMode = iota
	dEditDesc
	dInput
)

type inputKind int

const (
	ikPoints inputKind = iota
	ikTag
	ikComment
	ikCron
	ikAttach
)

// detailModel is the full-screen ticket view: a Glamour-rendered, scrollable
// description on the left, an editable metadata sidebar on the right, and the
// activity timeline below. Edits persist to the store immediately and the view
// reloads, so it stays consistent with concurrent agent changes.
type detailModel struct {
	store  *store.Store
	runMgr *run.Manager
	sched  *schedule.Scheduler
	theme  theme.Theme

	issueID int64
	issue   core.Issue

	statuses  []core.Status
	sprints   []core.Sprint
	epics     []core.Issue
	issueTags []core.Tag
	links     []store.LinkedIssue
	comments    []core.Comment
	runs        []core.Run
	schedules   []core.Schedule
	attachments []core.Attachment

	vp        viewport.Model
	desc      textarea.Model
	input     textinput.Model
	inputKind inputKind

	mode  detailMode
	field detailField

	width, height int
	err           string
	notice        string // transient confirmation (e.g. a launched/copied session), shown until the next key
}

func newDetail(st *store.Store, mgr *run.Manager, sched *schedule.Scheduler, th theme.Theme, issueID int64, w, h int) *detailModel {
	d := &detailModel{store: st, runMgr: mgr, sched: sched, theme: th, issueID: issueID, vp: viewport.New()}
	d.setSize(w, h)
	d.reload()
	return d
}

func (d *detailModel) setSize(w, h int) {
	d.width, d.height = w, h
	d.vp.SetWidth(d.descWidth())
	d.vp.SetHeight(d.descViewHeight())
	d.renderDescription()
}

// descViewHeight is the scrollable description region, leaving 2 lines in the
// left pane for the title.
func (d *detailModel) descViewHeight() int {
	h := d.bodyHeight() - 2
	if h < 2 {
		h = 2
	}
	return h
}

func (d *detailModel) descWidth() int {
	w := d.width*62/100 - 2
	if w < 20 {
		w = 20
	}
	return w
}

func (d *detailModel) bodyHeight() int {
	// header (1) + rule (1) + footer (1) + comments block (up to 6).
	h := d.height - 3 - d.commentsHeight()
	if h < 3 {
		h = 3
	}
	return h
}

func (d *detailModel) commentsHeight() int {
	n := len(d.comments)
	if n > 4 {
		n = 4
	}
	return n + 1 // title line
}

func (d *detailModel) reload() {
	iss, err := d.store.GetIssue(d.issueID)
	if err != nil {
		d.err = err.Error()
		return
	}
	d.issue = iss
	d.statuses, _ = d.store.ListStatuses(iss.ProjectID)
	d.sprints, _ = d.store.ListSprints(iss.ProjectID)
	d.epics, _ = d.store.ListIssues(store.IssueFilter{ProjectID: iss.ProjectID, Type: core.TypeEpic})
	d.issueTags, _ = d.store.ListIssueTags(iss.ID)
	d.links, _ = d.store.ListLinks(iss.ID)
	d.comments, _ = d.store.ListComments(iss.ID)
	d.runs, _ = d.store.ListRuns(iss.ID)
	d.schedules, _ = d.store.ListSchedules(iss.ID)
	d.attachments, _ = d.store.ListAttachments(iss.ID)
	d.vp.SetHeight(d.descViewHeight())
	d.renderDescription()
}

func (d *detailModel) renderDescription() {
	d.vp.SetContent(renderMarkdown(d.issue.Description, d.descWidth()))
}

// Update handles a message while the detail view is focused. It returns a
// command and whether to return to the board.
func (d *detailModel) Update(msg tea.Msg) (tea.Cmd, bool) {
	switch d.mode {
	case dEditDesc:
		return d.updateEditDesc(msg)
	case dInput:
		return d.updateInput(msg)
	default:
		return d.updateViewing(msg)
	}
}

func (d *detailModel) updateViewing(msg tea.Msg) (tea.Cmd, bool) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		d.vp, cmd = d.vp.Update(msg)
		return cmd, false
	}
	d.notice = "" // a fresh keypress clears any transient confirmation
	switch key.String() {
	case "esc", "q":
		return nil, true
	case "j":
		d.field = (d.field + 1) % dfFieldCount
	case "k":
		d.field = (d.field - 1 + dfFieldCount) % dfFieldCount
	case "l", "right":
		d.cycleField(+1)
	case "h", "left":
		d.cycleField(-1)
	case "up", "down", "pgup", "pgdown", "ctrl+u", "ctrl+d":
		var cmd tea.Cmd
		d.vp, cmd = d.vp.Update(msg)
		return cmd, false
	case "e":
		return d.startEditDesc(), false
	case "c":
		return d.startInput(ikComment, ""), false
	case "A":
		return d.startInput(ikAttach, ""), false
	case "o":
		d.openAttachment()
	case "s":
		d.startRun()
	case "D":
		d.startWithChildren()
	case "d":
		return d.discuss(), false
	case "S":
		return d.startInput(ikCron, ""), false
	case "w":
		d.toggleWorktree()
	case "X":
		d.unschedule()
	case "enter":
		if d.field == dfPoints {
			cur := ""
			if d.issue.StoryPoints != nil {
				cur = strconv.Itoa(*d.issue.StoryPoints)
			}
			return d.startInput(ikPoints, cur), false
		}
		if d.field == dfTags {
			return d.startInput(ikTag, ""), false
		}
	case "x":
		if d.field == dfTags && len(d.issueTags) > 0 {
			last := d.issueTags[len(d.issueTags)-1]
			if err := d.store.UntagIssue(d.issue.ID, last.ID); err != nil {
				d.err = err.Error()
			}
			d.reload()
		}
	}
	return nil, false
}

// startRun launches an agent on this ticket. The new run appears in the runs
// list, and the agent moves the ticket through its statuses over MCP.
func (d *detailModel) startRun() {
	if d.runMgr == nil {
		d.err = "run manager unavailable"
		return
	}
	if _, err := d.runMgr.Start(d.issue); err != nil {
		d.err = err.Error()
		return
	}
	d.err = ""
	d.reload()
}

// unschedule removes this ticket's most recent schedule (the one shown at the top
// of the sidebar list), so a mis-entered or no-longer-wanted cron can be undone.
func (d *detailModel) unschedule() {
	if d.sched == nil || len(d.schedules) == 0 {
		return
	}
	if err := d.sched.Remove(d.schedules[0].ID); err != nil {
		d.err = err.Error()
		return
	}
	d.err = ""
	d.reload()
}

// startWithChildren runs this ticket's children in dependency order, then the
// ticket itself.
func (d *detailModel) startWithChildren() {
	if d.runMgr == nil {
		d.err = "run manager unavailable"
		return
	}
	if err := d.runMgr.StartWithChildren(d.issue); err != nil {
		d.err = err.Error()
		return
	}
	d.err = ""
	d.reload()
}

// discuss opens an interactive agent session preloaded with this ticket in a new
// terminal (a multiplexer window or a GUI terminal) — never inline, so the board
// stays live. When no terminal can be reached it copies the command to the
// clipboard for the user to run themselves. The agent reflects any ticket
// changes back over MCP, which refresh the board live.
func (d *detailModel) discuss() tea.Cmd {
	if d.runMgr == nil {
		d.err = "run manager unavailable"
		return nil
	}
	cmd, err := d.runMgr.DiscussCommand(d.issue)
	if err != nil {
		d.err = err.Error()
		return nil
	}
	out := launchInTerminalOrCopy(cmd, "discussing "+d.issue.Key)
	if out.Err != nil {
		d.err = out.Err.Error()
		return nil
	}
	d.err, d.notice = "", out.Msg
	if out.Copy != "" {
		return tea.SetClipboard(out.Copy)
	}
	return nil
}

// toggleWorktree flips whether this ticket's runs execute in an isolated git
// worktree.
func (d *detailModel) toggleWorktree() {
	on := true
	if d.issue.WorktreeOn != nil {
		on = *d.issue.WorktreeOn
	}
	next := !on
	d.issue.WorktreeOn = &next
	d.saveIssue()
	d.reload()
}

// openAttachment opens the most recent attachment (the top of the sidebar list)
// in the user's default app or browser.
func (d *detailModel) openAttachment() {
	if len(d.attachments) == 0 {
		d.err = "no attachments to open"
		return
	}
	if err := openExternal(d.attachments[0].Path); err != nil {
		d.err = err.Error()
		return
	}
	d.err = ""
}

func (d *detailModel) startEditDesc() tea.Cmd {
	ta := textarea.New()
	ta.SetWidth(d.descWidth())
	ta.SetHeight(d.descViewHeight())
	ta.SetValue(d.issue.Description)
	d.desc = ta
	d.mode = dEditDesc
	return d.desc.Focus()
}

func (d *detailModel) updateEditDesc(msg tea.Msg) (tea.Cmd, bool) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			d.mode = dViewing
			return nil, false
		case "ctrl+s":
			d.issue.Description = d.desc.Value()
			if err := d.store.UpdateIssue(d.issue); err != nil {
				d.err = err.Error()
			}
			d.mode = dViewing
			d.reload()
			return nil, false
		}
	}
	var cmd tea.Cmd
	d.desc, cmd = d.desc.Update(msg)
	return cmd, false
}

func (d *detailModel) startInput(kind inputKind, value string) tea.Cmd {
	ti := textinput.New()
	ti.SetWidth(40)
	ti.SetValue(value)
	switch kind {
	case ikPoints:
		ti.Placeholder = "story points (number)"
	case ikTag:
		ti.Placeholder = "tag name"
	case ikComment:
		ti.Placeholder = "comment"
	case ikCron:
		ti.Placeholder = "cron e.g. 0 9 * * * (min hour dom mon dow)"
	case ikAttach:
		ti.Placeholder = "https://… or /path/to/file"
	}
	d.input = ti
	d.inputKind = kind
	d.mode = dInput
	return d.input.Focus()
}

func (d *detailModel) updateInput(msg tea.Msg) (tea.Cmd, bool) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			d.mode = dViewing
			return nil, false
		case "enter":
			d.submitInput(strings.TrimSpace(d.input.Value()))
			d.mode = dViewing
			d.reload()
			return nil, false
		}
	}
	var cmd tea.Cmd
	d.input, cmd = d.input.Update(msg)
	return cmd, false
}

func (d *detailModel) submitInput(value string) {
	switch d.inputKind {
	case ikPoints:
		if value == "" {
			d.issue.StoryPoints = nil
		} else if n, err := strconv.Atoi(value); err == nil && n >= 0 {
			d.issue.StoryPoints = &n
		} else {
			d.err = "story points must be a non-negative number"
			return
		}
		if err := d.store.UpdateIssue(d.issue); err != nil {
			d.err = err.Error()
		}
	case ikTag:
		if value == "" {
			return
		}
		tag, err := d.findOrCreateTag(value)
		if err != nil {
			d.err = err.Error()
			return
		}
		if err := d.store.TagIssue(d.issue.ID, tag.ID); err != nil {
			d.err = err.Error()
		}
	case ikComment:
		if value == "" {
			return
		}
		if _, err := d.store.AddComment(core.Comment{IssueID: d.issue.ID, Body: value}); err != nil {
			d.err = err.Error()
		}
	case ikCron:
		if value == "" {
			return
		}
		if d.sched == nil {
			d.err = "scheduler unavailable"
			return
		}
		if _, err := d.sched.Add(d.issue.ID, value, false); err != nil {
			d.err = err.Error()
		}
	case ikAttach:
		if value == "" {
			return
		}
		a := core.ClassifyAttachment(value)
		a.IssueID = d.issue.ID
		if !a.IsURL() {
			// Store an absolute path and the file size, so it opens regardless of
			// the cwd later and the size can be shown.
			if abs, err := filepath.Abs(value); err == nil {
				a.Path = abs
			}
			if fi, err := os.Stat(a.Path); err == nil {
				a.Size = fi.Size()
			}
		}
		if _, err := d.store.CreateAttachment(a); err != nil {
			d.err = err.Error()
		}
	}
}

func (d *detailModel) findOrCreateTag(name string) (core.Tag, error) {
	tags, err := d.store.ListTags(d.issue.ProjectID)
	if err != nil {
		return core.Tag{}, err
	}
	for _, t := range tags {
		if strings.EqualFold(t.Name, name) {
			return t, nil
		}
	}
	return d.store.CreateTag(core.Tag{ProjectID: d.issue.ProjectID, Name: name})
}
