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

// detailField enumerates the editable metadata values. The Properties panel
// owns the first group; the Agent panel owns the assignee triple. cycleField
// (detail_fields.go) acts on whichever one `field` points at.
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

// detailPanel is one focusable region of the ticket bento. TAB walks them and
// the focused one wears the iris border; arrows act within it. There are no
// tabs — every panel is on screen at once.
type detailPanel int

const (
	panelDescription detailPanel = iota
	panelProperties
	panelAgent
	panelRelations
	panelActivity
	panelCount
)

// propertyFields are the rows of the Properties panel, in display order.
func propertyFields() []detailField {
	return []detailField{dfStatus, dfType, dfPriority, dfPoints, dfSprint, dfEpic, dfTags}
}

// The Agent panel's rows: the assignee triple, the worktree toggle, then the
// three action buttons. agentSel indexes this list.
const (
	agProvider = iota
	agModel
	agEffort
	agWorktree
	agRun
	agDiscuss
	agSchedule
	agRowCount
)

// detailModel is the full-screen ticket view: a bento of bordered panels —
// Description, Properties, Agent, Relations & Files, Activity — each focusable
// with TAB. Edits persist to the store immediately and the view reloads, so it
// stays consistent with concurrent agent changes.
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
	mention   fileMention // inline "@" file picker, active only while editing the description

	mode          detailMode
	focus         detailPanel
	field         detailField // selected metadata field (Properties + the Agent triple)
	agentSel      int         // selected row in the Agent panel
	attachSel     int         // selected row in Relations & Files (attachments + the Attach button)
	commentScroll int         // Activity scroll offset

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
	d.renderDescription()
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
	if d.attachSel > len(d.attachments) {
		d.attachSel = len(d.attachments)
	}
	if d.attachSel < 0 {
		d.attachSel = 0
	}
	d.renderDescription()
}

func (d *detailModel) renderDescription() {
	iw := d.descInteriorWidth()
	d.vp.SetWidth(iw)
	d.vp.SetHeight(d.descViewportHeight())
	d.vp.SetContent(renderMarkdown(d.issue.Description, iw, d.repoRoot()))
}

// Update handles a message while the detail view is focused. It returns a
// command and whether to return to the active view.
func (d *detailModel) Update(msg tea.Msg) (tea.Cmd, bool) {
	if m, ok := msg.(repoFilesLoadedMsg); ok {
		if m.issueID == d.issueID {
			if m.err != nil {
				d.mention.load = mentionFailed
				d.mention.files = nil
			} else {
				d.mention.files = m.files
				d.mention.load = mentionReady
			}
			d.refreshMention()
		}
		return nil, false
	}
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
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		d.vp, cmd = d.vp.Update(msg)
		return cmd, false
	}
	d.notice = "" // a fresh keypress clears any transient confirmation
	switch k.String() {
	case "esc", "q":
		return nil, true
	case "tab":
		d.focusPanel(+1)
		return nil, false
	case "shift+tab":
		d.focusPanel(-1)
		return nil, false
	}
	switch d.focus {
	case panelDescription:
		return d.keyDescription(k)
	case panelProperties:
		return d.keyProperties(k)
	case panelAgent:
		return d.keyAgent(k)
	case panelRelations:
		return d.keyRelations(k)
	case panelActivity:
		return d.keyActivity(k)
	}
	return nil, false
}

// focusPanel moves focus to the next/previous panel and lands its internal
// cursor somewhere valid, so the arrow keys always have something to act on.
func (d *detailModel) focusPanel(dir int) {
	d.focus = detailPanel(wrap(int(d.focus)+dir, int(panelCount)))
	switch d.focus {
	case panelProperties:
		if idxOf(propertyFields(), d.field) < 0 {
			d.field = propertyFields()[0]
		}
	case panelAgent:
		if d.agentSel < 0 || d.agentSel >= agRowCount {
			d.agentSel = 0
		}
		d.syncAgentField()
	}
}

// syncAgentField points `field` at the metadata field under the Agent cursor, so
// cycleField changes the right value when the cursor is on the assignee triple.
func (d *detailModel) syncAgentField() {
	switch d.agentSel {
	case agProvider:
		d.field = dfProvider
	case agModel:
		d.field = dfModel
	case agEffort:
		d.field = dfEffort
	}
}

func (d *detailModel) keyDescription(k tea.KeyPressMsg) (tea.Cmd, bool) {
	switch k.String() {
	case "up", "down", "pgup", "pgdown", "ctrl+u", "ctrl+d", "j", "k":
		var cmd tea.Cmd
		d.vp, cmd = d.vp.Update(k)
		return cmd, false
	case "enter", "e":
		return d.startEditDesc(), false
	}
	return nil, false
}

func (d *detailModel) keyProperties(k tea.KeyPressMsg) (tea.Cmd, bool) {
	fs := propertyFields()
	switch k.String() {
	case "up", "k":
		d.field = fs[wrap(idxOf(fs, d.field)-1, len(fs))]
	case "down", "j":
		d.field = fs[wrap(idxOf(fs, d.field)+1, len(fs))]
	case "left", "h":
		d.cycleField(-1)
	case "right", "l":
		d.cycleField(+1)
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
	case "x", "backspace":
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

func (d *detailModel) keyAgent(k tea.KeyPressMsg) (tea.Cmd, bool) {
	d.syncAgentField() // keep `field` aligned with the cursor before any cycle
	switch k.String() {
	case "up", "k":
		d.agentSel = wrap(d.agentSel-1, agRowCount)
		d.syncAgentField()
	case "down", "j":
		d.agentSel = wrap(d.agentSel+1, agRowCount)
		d.syncAgentField()
	case "left", "h":
		if d.agentSel <= agEffort {
			d.cycleField(-1)
		} else if d.agentSel == agWorktree {
			d.toggleWorktree()
		}
	case "right", "l":
		if d.agentSel <= agEffort {
			d.cycleField(+1)
		} else if d.agentSel == agWorktree {
			d.toggleWorktree()
		}
	case "enter":
		switch d.agentSel {
		case agWorktree:
			d.toggleWorktree()
		case agRun:
			d.startRun()
		case agDiscuss:
			return d.discuss(), false
		case agSchedule:
			return d.startInput(ikCron, ""), false
		}
	}
	return nil, false
}

func (d *detailModel) keyRelations(k tea.KeyPressMsg) (tea.Cmd, bool) {
	n := len(d.attachments) // index n is the "+ Attach" button
	switch k.String() {
	case "up", "k":
		if d.attachSel > 0 {
			d.attachSel--
		}
	case "down", "j":
		if d.attachSel < n {
			d.attachSel++
		}
	case "enter", "o":
		if d.attachSel >= n {
			return d.startInput(ikAttach, ""), false
		}
		d.openAttachment()
	case "a":
		return d.startInput(ikAttach, ""), false
	}
	return nil, false
}

func (d *detailModel) keyActivity(k tea.KeyPressMsg) (tea.Cmd, bool) {
	switch k.String() {
	case "up", "k":
		if d.commentScroll > 0 {
			d.commentScroll--
		}
	case "down", "j":
		d.commentScroll++ // clamped against the window in renderActivity
	case "enter", "c":
		return d.startInput(ikComment, ""), false
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

// openAttachment opens the selected attachment in the user's default app or
// browser.
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
	ta.SetWidth(d.descInteriorWidth())
	ta.SetHeight(d.descViewportHeight())
	ta.SetValue(d.issue.Description)
	d.desc = ta
	d.mode = dEditDesc

	// Arm the inline "@" file picker by enumerating the repo off the update loop.
	// loadRepoFilesCmd falls back to the working directory when the project has no
	// usable repo path, so "@" works whenever Jeera runs inside a git repo.
	d.mention = fileMention{load: mentionPending}
	return tea.Batch(d.desc.Focus(), loadRepoFilesCmd(d.projectRepoPath(), d.issueID))
}

// projectRepoPath is the project's configured repo path — the preferred root for
// the "@" file picker. Empty when the project or its repo is unknown, in which
// case the loader falls back to the working directory.
func (d *detailModel) projectRepoPath() string {
	p, err := d.store.GetProject(d.issue.ProjectID)
	if err != nil {
		return ""
	}
	return p.RepoPath
}

// repoRoot resolves the directory that file references in the description are
// relative to: the project's repo path when it's a real directory, otherwise the
// directory Jeera was launched from. It mirrors the "@" picker's enumeration root
// so inserted links resolve back to the right files.
func (d *detailModel) repoRoot() string {
	if p := d.projectRepoPath(); p != "" {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return p
		}
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return ""
}

func (d *detailModel) updateEditDesc(msg tea.Msg) (tea.Cmd, bool) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		// While the "@" picker is open it owns navigation, accept and dismiss;
		// everything else (typing, backspace, left/right) falls through to the
		// textarea and then re-derives the token.
		if d.mention.active {
			switch key.String() {
			case "up", "ctrl+p":
				d.mention.move(-1)
				return nil, false
			case "down", "ctrl+n":
				d.mention.move(+1)
				return nil, false
			case "enter", "tab":
				d.acceptMention()
				return nil, false
			case "esc":
				d.closeMention()
				return nil, false
			}
		}
		switch key.String() {
		case "esc":
			d.mode = dViewing
			d.closeMention()
			return nil, false
		case "ctrl+s":
			d.issue.Description = d.desc.Value()
			if err := d.store.UpdateIssue(d.issue); err != nil {
				d.err = err.Error()
			}
			d.mode = dViewing
			d.closeMention()
			d.reload()
			return nil, false
		}
	}
	var cmd tea.Cmd
	d.desc, cmd = d.desc.Update(msg)
	d.refreshMention()
	return cmd, false
}

// refreshMention re-derives the "@" token from the textarea's buffer and caret
// after every edit, refiltering matches or closing the picker when the token is
// gone. The picker stays closed when the repo failed to load (so "@" is literal).
func (d *detailModel) refreshMention() {
	if d.mode != dEditDesc || d.mention.load == mentionFailed || d.mention.load == mentionIdle {
		d.mention.active = false
		return
	}
	line, col, ok := d.descCaretLine()
	if !ok {
		d.mention.active = false
		return
	}
	_, query, ok := activeMention(line, col)
	if !ok {
		d.mention.active = false
		return
	}
	if query != d.mention.query || !d.mention.active {
		d.mention.sel = 0
	}
	d.mention.query = query
	d.mention.matches = rankFiles(d.mention.files, query)
	if d.mention.sel >= len(d.mention.matches) {
		d.mention.sel = 0
	}
	d.mention.active = true
}

// acceptMention replaces the "@query" token under the caret with a Markdown link
// to the selected file. It deletes the token via the textarea (keeping the caret
// correct) and inserts the link in its place.
func (d *detailModel) acceptMention() {
	if len(d.mention.matches) == 0 || d.mention.sel >= len(d.mention.matches) {
		d.closeMention()
		return
	}
	line, col, ok := d.descCaretLine()
	if !ok {
		d.closeMention()
		return
	}
	at, _, ok := activeMention(line, col)
	if !ok {
		d.closeMention()
		return
	}
	rel := d.mention.matches[d.mention.sel]
	for i := 0; i < col-at; i++ { // delete "@" + query, left of the caret
		d.desc, _ = d.desc.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	d.desc.InsertString(markdownLink(rel))
	d.attachFile(rel)
	d.closeMention()
}

// attachFile records a picked repo file as an issue attachment (deduplicated) so
// it shows in the Relations & Files panel and can be opened from there. It
// resolves the repo-relative path to an absolute one and captures the size,
// mirroring the manual Attach flow (see submitInput/ikAttach).
func (d *detailModel) attachFile(rel string) {
	abs := rel
	if root := d.repoRoot(); root != "" {
		abs = filepath.Join(root, filepath.FromSlash(rel))
	}
	for _, a := range d.attachments { // already linked — don't add a duplicate
		if a.Path == abs {
			return
		}
	}
	a := core.ClassifyAttachment(abs)
	a.IssueID = d.issue.ID
	if fi, err := os.Stat(abs); err == nil {
		a.Size = fi.Size()
	}
	if _, err := d.store.CreateAttachment(a); err != nil {
		d.err = err.Error()
		return
	}
	d.attachments, _ = d.store.ListAttachments(d.issue.ID) // refresh the panel now
}

// closeMention dismisses the picker while keeping the loaded file list for reuse
// within the same edit session.
func (d *detailModel) closeMention() {
	d.mention.active = false
	d.mention.query = ""
	d.mention.matches = nil
	d.mention.sel = 0
}

// descCaretLine returns the runes of the textarea's current logical line and the
// caret's column within it.
func (d *detailModel) descCaretLine() (line []rune, col int, ok bool) {
	row := d.desc.Line()
	lines := strings.Split(d.desc.Value(), "\n")
	if row < 0 || row >= len(lines) {
		return nil, 0, false
	}
	return []rune(lines[row]), d.desc.Column(), true
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
