// Package tui is Jeera's terminal board: a Bubble Tea v2 application rendered
// from the design system in tui/theme. It reads and writes the same store the
// embedded MCP server uses and refreshes live when an agent changes data.
package tui

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/03-CiprianoG/jeera/internal/config"
	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/mcp"
	"github.com/03-CiprianoG/jeera/internal/run"
	"github.com/03-CiprianoG/jeera/internal/schedule"
	"github.com/03-CiprianoG/jeera/internal/store"
	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

// mode is the transient layer on top of the active view: an overlay, the
// full-screen detail, or modeNormal (nothing layered — show the active view).
type mode int

const (
	modeNormal mode = iota // no overlay; render the active view
	modeForm
	modeHelp
	modeMCP
	modeProjects
	modeConfirm
	modeDetail
	modePicker
	modeSettings
	modeSearch // the find overlay over the Board or Backlog
)

// view is the active top-level destination shown in the navbar. Unlike mode, a
// view persists beneath any overlay: dismissing an overlay returns to whichever
// view was showing, so the three destinations behave as peers rather than as
// detours off the board.
type view int

const (
	viewBoard view = iota
	viewBacklog
	viewSprints
	viewRuns
	viewCount
)

// Model is the root Bubble Tea model.
type Model struct {
	store  *store.Store
	mcp    *mcp.Server // nil when started with --no-mcp
	runMgr *run.Manager
	sched  *schedule.Scheduler // nil when scheduling is unavailable
	cfg    *config.Store
	theme  theme.Theme
	keys   keyMap

	settings *settingsModel // non-nil while the settings view is open

	width, height int

	projects   []core.Project
	active     core.Project
	board      boardData
	backlog    backlogData
	sprints    sprintsData
	activeRuns int
	recentRuns []core.Run
	runsCursor int // selected row in the Runs view

	colIdx, cardIdx int
	backlogSel      int // selected issue in the Backlog view
	sprintSel       int // selected row (sprint header or issue) in the Sprints view

	view      view
	mode      mode
	form      *formModel
	detail    *detailModel
	picker    *pickerModel // non-nil while a chooser overlay is open
	search    *searchModel // non-nil while the find overlay is open
	confirm   string
	onConfirm func() tea.Cmd
	projSel   int

	// Live search filters, one per searchable view, persisted across the store-
	// event reloads so an agent's change never silently drops the filter. The
	// *Total fields hold the unfiltered count, so the chrome can say "3 of 12".
	boardQuery, backlogQuery string
	boardTotal, backlogTotal int

	toastText string
	errText   string
}

// New builds the root model over a store, an optional running MCP server, the
// run manager that starts agents, the scheduler that fires timed runs, and the
// live settings store. A nil cfg falls back to an on-disk store at the default
// path, so callers (and tests) need not always supply one.
func New(st *store.Store, mcpSrv *mcp.Server, mgr *run.Manager, sched *schedule.Scheduler, cfg *config.Store) Model {
	if cfg == nil {
		cfg, _ = config.NewStore(config.Path())
	}
	m := Model{
		store:  st,
		mcp:    mcpSrv,
		runMgr: mgr,
		sched:  sched,
		cfg:    cfg,
		theme:  theme.New(),
		keys:   newKeyMap(),
	}
	// Open on the project the user pinned as default, when one is set and still
	// exists. reload() keeps any non-zero active project it finds, and falls back
	// to the oldest project on its own when the pin is stale or unset.
	if prefix := cfg.Get().DefaultProjectPrefix; prefix != "" {
		if p, err := st.GetProjectByPrefix(prefix); err == nil {
			m.active = p
		}
	}
	m.reload()
	return m
}

// Init implements tea.Model. It emits the initial terminal title so the tab
// reflects the active project from the first frame.
func (m Model) Init() tea.Cmd { return emitTabTitle(m.windowTitle()) }

// reload re-reads projects and the active project's board from the store. It
// preserves the current selection by issue ID, so the asynchronous store-event
// reload (fired after every mutation, including the TUI's own) does not throw
// the highlight off the card a move/create/rename just acted on.
func (m *Model) reload() {
	prevID := int64(0)
	if iss, ok := m.selectedIssue(); ok {
		prevID = iss.ID
	}

	projects, err := m.store.ListProjects()
	if err != nil {
		m.errText = err.Error()
		return
	}
	m.projects = projects
	if active, err := m.store.ListActiveRuns(); err == nil {
		m.activeRuns = len(active)
	}

	if m.active.ID != 0 {
		found := false
		for _, p := range projects {
			if p.ID == m.active.ID {
				m.active = p
				found = true
				break
			}
		}
		if !found {
			m.active = core.Project{}
		}
	}
	if m.active.ID == 0 && len(projects) > 0 {
		m.active = projects[0]
	}
	if m.active.ID == 0 {
		m.board = boardData{}
		m.refreshView()
		return
	}

	bd, err := loadBoard(m.store, m.active.ID)
	if err != nil {
		m.errText = err.Error()
		return
	}
	// A live board search narrows the lanes here, at the single load chokepoint,
	// so every downstream invariant (selection, move, count) holds over exactly
	// what's shown — and the filter survives the post-mutation reload untouched.
	m.boardTotal = countCards(bd)
	if m.boardQuery != "" {
		bd = filterBoard(bd, m.boardQuery)
	}
	m.board = bd
	if prevID != 0 {
		m.selectIssueByID(prevID) // re-anchor; falls back to clamp if it's gone
	} else {
		m.clampSelection()
	}
	m.refreshView()
}

// refreshView reloads the auxiliary data backing the active non-board view, so
// Sprints and Runs update live alongside the board after any store change. It is
// a no-op on the board (whose data reload() already handles).
func (m *Model) refreshView() {
	switch m.view {
	case viewBacklog:
		prevID := int64(0)
		if iss, ok := m.selectedBacklogIssue(); ok {
			prevID = iss.ID
		}
		bl, err := loadBacklog(m.store, m.active.ID)
		if err != nil {
			m.errText = err.Error()
			return
		}
		// Keep the true backlog size for the chrome, then narrow to the live
		// filter so selection and rendering work over exactly what's shown.
		m.backlogTotal = len(bl.issues)
		if m.backlogQuery != "" {
			bl.issues = filterIssues(bl.issues, m.backlogQuery)
		}
		m.backlog = bl
		if prevID != 0 {
			for i, iss := range m.backlog.issues {
				if iss.ID == prevID {
					m.backlogSel = i
					break
				}
			}
		}
		m.clampBacklogSel()
	case viewSprints:
		// Re-anchor the cursor by what it's on — a sprint header or an issue — so a
		// live agent change that reorders the list doesn't slide the highlight onto a
		// different row than the one the user selected.
		var wantSprint, wantIssue int64
		if it, ok := m.selectedSprintItem(); ok {
			if it.kind == itemHeader {
				wantSprint = it.sprint.ID
			} else {
				wantIssue = it.issue.ID
			}
		}
		sp, err := loadSprints(m.store, m.active.ID)
		if err != nil {
			m.errText = err.Error()
			return
		}
		m.sprints = sp
		for i, it := range m.sprints.items() {
			if (it.kind == itemHeader && wantSprint != 0 && it.sprint.ID == wantSprint) ||
				(it.kind == itemIssue && wantIssue != 0 && it.issue.ID == wantIssue) {
				m.sprintSel = i
				break
			}
		}
		m.clampSprintSel()
	case viewRuns:
		// Re-anchor the cursor by run ID: a run starting under an open Runs view
		// prepends to the list, which would otherwise slide the cursor onto a
		// different run.
		prevID := int64(0)
		if m.runsCursor >= 0 && m.runsCursor < len(m.recentRuns) {
			prevID = m.recentRuns[m.runsCursor].ID
		}
		m.recentRuns, _ = m.store.ListRecentRuns(50)
		m.runsCursor = 0
		for i, r := range m.recentRuns {
			if r.ID == prevID {
				m.runsCursor = i
				break
			}
		}
		m.clampRunsCursor()
	}
}

// switchView moves the active destination by delta (wrapping) and loads its data.
func (m *Model) switchView(delta int) {
	m.view = view((int(m.view) + delta + int(viewCount)) % int(viewCount))
	m.refreshView()
}

// renderActiveView renders the body of the active destination into a region
// exactly height rows tall.
func (m Model) renderActiveView(height int) string {
	switch m.view {
	case viewBacklog:
		return m.renderBacklog(height)
	case viewSprints:
		return m.renderSprints(height)
	case viewRuns:
		return m.renderRuns(height)
	default:
		return m.renderBoard(height)
	}
}

func (m *Model) clampSelection() {
	if len(m.board.columns) == 0 {
		m.colIdx, m.cardIdx = 0, 0
		return
	}
	if m.colIdx >= len(m.board.columns) {
		m.colIdx = len(m.board.columns) - 1
	}
	if m.colIdx < 0 {
		m.colIdx = 0
	}
	// cardIdx ranges 0..n: on the To Do column n (one past the last card) is the
	// "+ New issue" slot. Columns without that slot clamp to their last real card.
	n := len(m.board.columns[m.colIdx].cards)
	if !m.columnHasAddCard(m.colIdx) && n > 0 {
		n--
	}
	if m.cardIdx > n {
		m.cardIdx = n
	}
	if m.cardIdx < 0 {
		m.cardIdx = 0
	}
}

// clampRunsCursor keeps the Runs-overlay selection within the live run list,
// which changes under it as agents start and finish.
func (m *Model) clampRunsCursor() {
	if m.runsCursor >= len(m.recentRuns) {
		m.runsCursor = len(m.recentRuns) - 1
	}
	if m.runsCursor < 0 {
		m.runsCursor = 0
	}
}

// selectedIssue returns the currently highlighted issue, if any.
func (m Model) selectedIssue() (core.Issue, bool) {
	if m.colIdx < 0 || m.colIdx >= len(m.board.columns) {
		return core.Issue{}, false
	}
	cards := m.board.columns[m.colIdx].cards
	if m.cardIdx < 0 || m.cardIdx >= len(cards) {
		return core.Issue{}, false
	}
	return cards[m.cardIdx], true
}

// Update implements tea.Model. It routes the message, then keeps the terminal
// tab in sync: whenever the active project (and thus the title) changes, it
// emits an OSC 0 sequence. Bubble Tea's View.WindowTitle only emits OSC 2 (the
// window title), which many terminals show in the title bar but not the tab.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	nm, ok := next.(Model)
	if !ok {
		return next, cmd
	}
	// Only an update that actually flips the title needs to re-emit it; comparing
	// before vs after keeps unrelated updates returning their command untouched.
	if after := nm.windowTitle(); after != m.windowTitle() {
		cmd = tea.Batch(cmd, emitTabTitle(after))
	}
	return nm, cmd
}

// emitTabTitle returns a command that writes an OSC 0 sequence, setting both the
// terminal's icon name (the tab) and window title. This complements Bubble Tea's
// own OSC 2 (window-title-only) emission so the tab label updates too.
func emitTabTitle(title string) tea.Cmd {
	return func() tea.Msg {
		fmt.Fprint(os.Stdout, ansi.SetIconNameWindowTitle(title))
		return nil
	}
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.detail != nil {
			m.detail.setSize(m.width, m.height)
		}
		return m, nil
	case storeEventMsg:
		m.reload() // also refreshes the active Sprints/Runs view
		if m.mode == modeDetail && m.detail != nil {
			m.detail.reload()
		}
		return m, nil
	case errMsg:
		m.errText = msg.err.Error()
		m.toastText = ""
		return m, nil
	case toastMsg:
		m.toastText = msg.text
		m.errText = ""
		return m, nil
	case clearToastMsg:
		m.toastText = ""
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	// Route other messages (e.g. cursor blink) to the active form, search or
	// detail view.
	if m.mode == modeForm && m.form != nil {
		return m, m.form.update(msg)
	}
	if m.mode == modeSearch && m.search != nil {
		return m, m.search.update(msg)
	}
	if m.mode == modeDetail && m.detail != nil {
		return m.routeDetail(msg)
	}
	return m, nil
}

// routeDetail forwards a message to the detail view and returns to the active
// view when it signals done.
func (m Model) routeDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.detail == nil {
		m.mode = modeNormal
		return m, nil
	}
	cmd, back := m.detail.Update(msg)
	if back {
		m.mode = modeNormal
		m.detail = nil
		m.reload()
	}
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.mode {
	case modeForm:
		return m.updateForm(msg)
	case modeHelp, modeMCP:
		m.mode = modeNormal // any key dismisses the overlay
		return m, nil
	case modeProjects:
		return m.updateProjects(msg)
	case modeConfirm:
		return m.updateConfirm(msg)
	case modeDetail:
		return m.routeDetail(msg)
	case modePicker:
		return m.updatePicker(msg)
	case modeSearch:
		return m.updateSearch(msg)
	case modeSettings:
		if m.settings != nil && m.settings.update(msg) {
			m.settings = nil
			m.mode = modeNormal
		}
		return m, nil
	}
	// modeNormal: global keys (view-switch + overlays) win first, so they work
	// from every view; then the active view handles its own navigation.
	if next, cmd, handled := m.handleGlobalKey(msg); handled {
		return next, cmd
	}
	switch m.view {
	case viewBacklog:
		return m.updateBacklog(msg)
	case viewSprints:
		return m.updateSprints(msg)
	case viewRuns:
		return m.updateRuns(msg)
	default:
		return m.updateBoard(msg)
	}
}

// windowTitle is the terminal window/process title: "Jeera - {Project}" when a
// project is active, falling back to "Jeera" before any project is loaded.
func (m Model) windowTitle() string {
	if m.active.Name != "" {
		return "Jeera - " + m.active.Name
	}
	return "Jeera"
}

// View implements tea.Model.
func (m Model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		v := tea.NewView("")
		v.WindowTitle = m.windowTitle()
		return v
	}
	if m.width < 30 || m.height < 8 {
		v := tea.NewView(m.theme.HelpDesc.Render("terminal too small"))
		v.AltScreen = true
		v.BackgroundColor = m.theme.P.BgBase
		v.WindowTitle = m.windowTitle()
		return v
	}
	// The detail view takes over the whole screen.
	if m.mode == modeDetail && m.detail != nil {
		v := tea.NewView(m.detail.View())
		v.AltScreen = true
		v.BackgroundColor = m.theme.P.BgBase
		v.WindowTitle = m.windowTitle()
		return v
	}
	nav := m.renderNavbar()
	footer := m.renderFooter()
	midHeight := m.height - lipgloss.Height(nav) - lipgloss.Height(footer)
	if midHeight < 1 {
		midHeight = 1
	}

	var mid string
	switch m.mode {
	case modeHelp:
		mid = m.center(m.renderHelp(), midHeight)
	case modeMCP:
		mid = m.center(m.renderMCP(), midHeight)
	case modeForm:
		mid = m.center(m.form.View(m.theme), midHeight)
	case modeProjects:
		mid = m.center(m.renderProjects(), midHeight)
	case modeConfirm:
		mid = m.center(m.renderConfirm(), midHeight)
	case modePicker:
		mid = m.center(m.picker.View(m.theme), midHeight)
	case modeSearch:
		mid = m.center(m.renderSearch(), midHeight)
	case modeSettings:
		mid = m.center(m.settings.View(), midHeight)
	default: // modeNormal
		mid = m.renderActiveView(midHeight)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, nav, mid, footer)
	v := tea.NewView(content)
	v.AltScreen = true
	v.BackgroundColor = m.theme.P.BgBase
	v.WindowTitle = m.windowTitle()
	return v
}

func (m Model) center(s string, height int) string {
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, s)
}
