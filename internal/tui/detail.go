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
	"charm.land/lipgloss/v2"

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

// detailTab is the active section of the ticket view. The tabs split the ticket
// the way the navbar splits the app — Tab/Shift+Tab move between them — and the
// tab strip uses the same treatment as the navbar so the two read as one system.
type detailTab int

const (
	tabOverview  detailTab = iota // description + the core metadata fields
	tabAgent                      // who runs it, its runs and schedules
	tabRelations                  // epic, parent, children and links
	tabFiles                      // attachments
	tabActivity                   // the comment timeline
	tabCount
)

var detailTabLabels = []string{"Overview", "Agent", "Relations", "Files", "Activity"}

// fields lists the cyclable metadata fields shown on a tab, in display order.
// Only Overview and Agent carry fields; the others are read/act views.
func (t detailTab) fields() []detailField {
	switch t {
	case tabOverview:
		return []detailField{dfStatus, dfType, dfPriority, dfPoints, dfSprint, dfEpic, dfTags}
	case tabAgent:
		return []detailField{dfProvider, dfModel, dfEffort}
	default:
		return nil
	}
}

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

	statuses    []core.Status
	sprints     []core.Sprint
	epics       []core.Issue
	issueTags   []core.Tag
	links       []store.LinkedIssue
	children    []core.Issue
	parent      *core.Issue
	comments    []core.Comment
	runs        []core.Run
	schedules   []core.Schedule
	attachments []core.Attachment

	vp        viewport.Model
	desc      textarea.Model
	input     textinput.Model
	inputKind inputKind

	mode      detailMode
	tab       detailTab
	field     detailField
	attachSel int // selected attachment on the Files tab

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

// bodyHeight is the active tab's content region: the screen minus the header (1),
// the tab strip and the footer (1). The strip can wrap to more than one visual row
// on a narrow terminal, so we measure it rather than assume two rows — that keeps
// the footer on screen at every width.
func (d *detailModel) tabStripHeight() int {
	return lipgloss.Height(tabStrip(d.theme, d.width, detailTabLabels, int(d.tab)))
}

func (d *detailModel) bodyHeight() int {
	h := d.height - 2 - d.tabStripHeight()
	if h < 3 {
		h = 3
	}
	return h
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
	id := iss.ID
	d.children, _ = d.store.ListIssues(store.IssueFilter{ProjectID: iss.ProjectID, ParentID: &id})
	d.parent = nil
	if iss.ParentID != nil {
		if p, err := d.store.GetIssue(*iss.ParentID); err == nil {
			d.parent = &p
		}
	}
	d.comments, _ = d.store.ListComments(iss.ID)
	d.runs, _ = d.store.ListRuns(iss.ID)
	d.schedules, _ = d.store.ListSchedules(iss.ID)
	d.attachments, _ = d.store.ListAttachments(iss.ID)
	if d.attachSel >= len(d.attachments) {
		d.attachSel = len(d.attachments) - 1
	}
	if d.attachSel < 0 {
		d.attachSel = 0
	}
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
	case "tab":
		d.switchTab(+1)
	case "shift+tab":
		d.switchTab(-1)
	case "j":
		d.moveCursor(+1)
	case "k":
		d.moveCursor(-1)
	case "l", "right":
		if len(d.tab.fields()) > 0 {
			d.cycleField(+1)
		}
	case "h", "left":
		if len(d.tab.fields()) > 0 {
			d.cycleField(-1)
		}
	case "up", "down", "pgup", "pgdown", "ctrl+u", "ctrl+d":
		if d.tab == tabOverview { // the description is the only scrollable pane
			var cmd tea.Cmd
			d.vp, cmd = d.vp.Update(msg)
			return cmd, false
		}
	case "e":
		if d.tab == tabOverview {
			return d.startEditDesc(), false
		}
	case "c":
		if d.tab == tabActivity {
			return d.startInput(ikComment, ""), false
		}
	case "A":
		if d.tab == tabFiles {
			return d.startInput(ikAttach, ""), false
		}
	case "o":
		if d.tab == tabFiles {
			d.openAttachment()
		}
	case "s":
		if d.tab == tabAgent {
			d.startRun()
		}
	case "D":
		if d.tab == tabAgent {
			d.startWithChildren()
		}
	case "d":
		if d.tab == tabAgent {
			return d.discuss(), false
		}
	case "S":
		if d.tab == tabAgent {
			return d.startInput(ikCron, ""), false
		}
	case "w":
		if d.tab == tabAgent {
			d.toggleWorktree()
		}
	case "X":
		if d.tab == tabAgent {
			d.unschedule()
		}
	case "enter":
		if d.tab == tabOverview && d.field == dfPoints {
			cur := ""
			if d.issue.StoryPoints != nil {
				cur = strconv.Itoa(*d.issue.StoryPoints)
			}
			return d.startInput(ikPoints, cur), false
		}
		if d.tab == tabOverview && d.field == dfTags {
			return d.startInput(ikTag, ""), false
		}
	case "x":
		if d.tab == tabOverview && d.field == dfTags && len(d.issueTags) > 0 {
			last := d.issueTags[len(d.issueTags)-1]
			if err := d.store.UntagIssue(d.issue.ID, last.ID); err != nil {
				d.err = err.Error()
			}
			d.reload()
		}
	}
	return nil, false
}

// switchTab moves to the next/previous tab and lands the field cursor on the new
// tab's first field, so h/l always has something valid to cycle.
func (d *detailModel) switchTab(dir int) {
	d.tab = detailTab(wrap(int(d.tab)+dir, int(tabCount)))
	if fs := d.tab.fields(); len(fs) > 0 {
		d.field = fs[0]
	}
}

// moveCursor walks the current tab's selectable rows: the metadata fields on
// Overview/Agent, the attachment list on Files. Other tabs have no row cursor.
func (d *detailModel) moveCursor(dir int) {
	switch d.tab {
	case tabOverview, tabAgent:
		fs := d.tab.fields()
		if len(fs) == 0 {
			return
		}
		cur := idxOf(fs, d.field)
		if cur < 0 {
			cur = 0
		}
		d.field = fs[wrap(cur+dir, len(fs))]
	case tabFiles:
		if len(d.attachments) > 0 {
			d.attachSel = wrap(d.attachSel+dir, len(d.attachments))
		}
	}
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

// selectedAttachment returns the attachment under the Files cursor, if any.
func (d *detailModel) selectedAttachment() (core.Attachment, bool) {
	if d.attachSel < 0 || d.attachSel >= len(d.attachments) {
		return core.Attachment{}, false
	}
	return d.attachments[d.attachSel], true
}

// openAttachment opens the selected attachment (on the Files tab) in the user's
// default app or browser.
func (d *detailModel) openAttachment() {
	a, ok := d.selectedAttachment()
	if !ok {
		d.err = "no attachments to open"
		return
	}
	if err := openExternal(a.Path); err != nil {
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
