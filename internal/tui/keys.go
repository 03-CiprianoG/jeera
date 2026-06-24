package tui

import "charm.land/bubbles/v2/key"

// keyMap is the global keymap and the single source of truth for the footer and
// the help overlay, which render straight from these bindings.
type keyMap struct {
	Up        key.Binding
	Down      key.Binding
	Left      key.Binding
	Right     key.Binding
	MoveLeft  key.Binding
	MoveRight key.Binding
	New       key.Binding
	Edit      key.Binding
	Delete    key.Binding
	Enter     key.Binding
	NextView  key.Binding
	PrevView  key.Binding
	Project   key.Binding
	MCP       key.Binding
	Resume    key.Binding
	Watch     key.Binding
	Settings  key.Binding
	Refresh   key.Binding
	Help      key.Binding
	Quit      key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Left:      key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "prev column")),
		Right:     key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next column")),
		MoveLeft:  key.NewBinding(key.WithKeys("H", "shift+left"), key.WithHelp("H", "move left")),
		MoveRight: key.NewBinding(key.WithKeys("L", "shift+right"), key.WithHelp("L", "move right")),
		New:       key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
		Edit:      key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "rename")),
		Delete:    key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "delete")),
		Enter:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		NextView:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next view")),
		PrevView:  key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("⇧tab", "prev view")),
		Project:   key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "projects")),
		MCP:       key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "mcp")),
		Resume:    key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "resume")),
		Watch:     key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "watch")),
		Settings:  key.NewBinding(key.WithKeys(","), key.WithHelp(",", "settings")),
		Refresh:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// ShortHelp is the one-line summary for the help.KeyMap interface.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.NextView, k.Up, k.Down, k.Enter, k.New, k.Help, k.Quit}
}

// navHints are the footer key hints for the active view, in priority order
// (earlier ones survive a tight width). The view switcher leads on every view so
// Tab is always discoverable; the rest are the keys that view actually responds
// to, so the footer never advertises an action the current screen can't take.
func (m Model) navHints() []key.Binding {
	overlays := []key.Binding{m.keys.Project, m.keys.MCP, m.keys.Settings, m.keys.Refresh}
	switch m.view {
	case viewSprints:
		hints := []key.Binding{m.keys.NextView}
		if len(m.sprints.flatIssues()) > 0 { // only offer actions there's something to act on
			hints = append(hints, m.keys.Up, m.keys.Down, m.keys.Enter)
		}
		return append(hints, overlays...)
	case viewRuns:
		hints := []key.Binding{m.keys.NextView}
		if len(m.recentRuns) > 0 {
			hints = append(hints, m.keys.Up, m.keys.Down, m.keys.Resume, m.keys.Watch)
		}
		return append(hints, overlays...)
	default: // board
		if m.active.ID == 0 { // welcome screen: the only real action is creating a project
			return append([]key.Binding{m.keys.NextView, m.keys.New}, overlays...)
		}
		hints := []key.Binding{m.keys.NextView, m.keys.Up, m.keys.Down, m.keys.Left, m.keys.Right, m.keys.New}
		if _, ok := m.selectedIssue(); ok { // card-specific actions need something selected
			hints = append(hints, m.keys.Edit, m.keys.Delete, m.keys.Enter, m.keys.MoveLeft, m.keys.MoveRight)
		}
		return append(hints, overlays...)
	}
}

// FullHelp is the expanded help overlay. Four columns so the modal fits a
// standard-width terminal without overflowing.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.NextView, k.PrevView, k.Up, k.Down, k.Left, k.Right},
		{k.New, k.Edit, k.Delete, k.Enter, k.MoveLeft, k.MoveRight},
		{k.Resume, k.Watch, k.Project, k.MCP},
		{k.Settings, k.Refresh, k.Help, k.Quit},
	}
}
