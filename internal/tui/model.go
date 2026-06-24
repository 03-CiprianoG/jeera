// Package tui is Jeera's terminal board: a Bubble Tea v2 application rendered
// from the design system in tui/theme. It reads and writes the same store the
// embedded MCP server uses and refreshes live when an agent changes data.
package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/mcp"
	"github.com/03-CiprianoG/jeera/internal/store"
	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

type mode int

const (
	modeBoard mode = iota
	modeForm
	modeHelp
	modeMCP
	modeProjects
	modeConfirm
)

// Model is the root Bubble Tea model.
type Model struct {
	store *store.Store
	mcp   *mcp.Server // nil when started with --no-mcp
	theme theme.Theme
	keys  keyMap

	width, height int

	projects []core.Project
	active   core.Project
	board    boardData

	colIdx, cardIdx int

	mode      mode
	form      *formModel
	confirm   string
	onConfirm func() tea.Cmd
	projSel   int

	toastText string
	errText   string
}

// New builds the root model over a store and (optionally) a running MCP server.
func New(st *store.Store, mcpSrv *mcp.Server) Model {
	m := Model{
		store: st,
		mcp:   mcpSrv,
		theme: theme.New(),
		keys:  newKeyMap(),
	}
	m.reload()
	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// reload re-reads projects and the active project's board from the store.
func (m *Model) reload() {
	projects, err := m.store.ListProjects()
	if err != nil {
		m.errText = err.Error()
		return
	}
	m.projects = projects

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
		return
	}

	bd, err := loadBoard(m.store, m.active.ID)
	if err != nil {
		m.errText = err.Error()
		return
	}
	m.board = bd
	m.clampSelection()
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
	n := len(m.board.columns[m.colIdx].cards)
	if m.cardIdx >= n {
		m.cardIdx = n - 1
	}
	if m.cardIdx < 0 {
		m.cardIdx = 0
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

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case storeEventMsg:
		m.reload()
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
	// Route other messages (e.g. cursor blink) to the active form.
	if m.mode == modeForm && m.form != nil {
		return m, m.form.update(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.mode {
	case modeForm:
		return m.updateForm(msg)
	case modeHelp, modeMCP:
		m.mode = modeBoard // any key dismisses the overlay
		return m, nil
	case modeProjects:
		return m.updateProjects(msg)
	case modeConfirm:
		return m.updateConfirm(msg)
	default:
		return m.updateBoard(msg)
	}
}

// View implements tea.Model.
func (m Model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return tea.NewView("")
	}
	header := m.renderHeader()
	footer := m.renderFooter()
	midHeight := m.height - lipgloss.Height(header) - lipgloss.Height(footer)
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
	default:
		mid = m.renderBoard(midHeight)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, header, mid, footer)
	v := tea.NewView(content)
	v.AltScreen = true
	v.BackgroundColor = m.theme.P.BgBase
	return v
}

func (m Model) center(s string, height int) string {
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, s)
}
