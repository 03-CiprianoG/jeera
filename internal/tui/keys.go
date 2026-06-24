package tui

import "charm.land/bubbles/v2/key"

// keyMap is the global keymap. It implements help.KeyMap so the footer and the
// help overlay render straight from these bindings — one source of truth.
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
	Project   key.Binding
	MCP       key.Binding
	Runs      key.Binding
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
		Project:   key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "projects")),
		MCP:       key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "mcp")),
		Runs:      key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "runs")),
		Settings:  key.NewBinding(key.WithKeys(","), key.WithHelp(",", "settings")),
		Refresh:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// ShortHelp is the one-line footer.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Left, k.Right, k.Up, k.Down, k.New, k.MoveLeft, k.MoveRight, k.Help, k.Quit}
}

// navHints are the bindings shown in the footer before the reserved help/quit
// tail, in priority order (earlier ones survive a tight width).
func (m Model) navHints() []key.Binding {
	return []key.Binding{
		m.keys.Left, m.keys.Right, m.keys.Up, m.keys.Down,
		m.keys.New, m.keys.Edit, m.keys.Delete, m.keys.Enter,
		m.keys.MoveLeft, m.keys.MoveRight, m.keys.Project, m.keys.MCP, m.keys.Runs, m.keys.Settings, m.keys.Refresh,
	}
}

// FullHelp is the expanded help overlay, grouped into columns.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.New, k.Edit, k.Delete, k.Enter},
		{k.MoveLeft, k.MoveRight, k.Project},
		{k.MCP, k.Runs, k.Settings, k.Refresh, k.Help, k.Quit},
	}
}
