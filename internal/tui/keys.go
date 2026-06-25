package tui

import "charm.land/bubbles/v2/key"

// keyMap is the global keymap and the single source of truth for the help
// overlay, which renders straight from these bindings. Jeera is direct-
// manipulation first: views switch with ⌥tab, tickets move with ⇧+arrows, and
// tab is reserved for moving focus *inside* a view (form fields, ticket panels).
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
	Assign    key.Binding
	Cycle     key.Binding
	Search    key.Binding
	Unsprint  key.Binding
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
		Up:    key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑", "up")),
		Down:  key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓", "down")),
		Left:  key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←", "prev column")),
		Right: key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→", "next column")),
		// Moving a ticket across columns is the board's one direct gesture: hold
		// shift and steer. The vim H/L stay bound for muscle memory.
		MoveLeft:  key.NewBinding(key.WithKeys("shift+left", "H"), key.WithHelp("⇧←", "move ticket")),
		MoveRight: key.NewBinding(key.WithKeys("shift+right", "L"), key.WithHelp("⇧→", "move ticket")),
		New:       key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
		Edit:      key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "rename")),
		Delete:    key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "delete")),
		Enter:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Assign:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "assign")),
		Cycle:     key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "start/finish")),
		// Find on the Board and Backlog. ⌘F (super+f) is the headline — it stands in
		// for the terminal's own find on terminals that speak the Kitty keyboard
		// protocol and don't reserve the Command key (Bubble Tea requests key
		// disambiguation by default, so they forward it). "/" is the universal
		// fallback for terminals — like Apple Terminal — that always eat ⌘F.
		Search:   key.NewBinding(key.WithKeys("super+f", "/"), key.WithHelp("⌘f /", "search")),
		Unsprint: key.NewBinding(key.WithKeys("backspace"), key.WithHelp("⌫", "to backlog")),
		// ⌥tab (option+tab) walks the navbar; tab itself is left for in-view focus.
		NextView: key.NewBinding(key.WithKeys("alt+tab"), key.WithHelp("⌥tab", "next view")),
		PrevView: key.NewBinding(key.WithKeys("alt+shift+tab"), key.WithHelp("⌥⇧tab", "prev view")),
		Project:  key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "projects")),
		MCP:      key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "mcp")),
		Resume:   key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "resume")),
		Watch:    key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "watch")),
		Settings: key.NewBinding(key.WithKeys(","), key.WithHelp(",", "settings")),
		Refresh:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// FullHelp is the expanded help overlay, grouped the way the app is used:
// wayfinding, the board's direct gestures, creating and running work, then the
// global utilities.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.NextView, k.PrevView, k.Up, k.Down, k.Left, k.Right},
		{k.Enter, k.Search, k.Edit, k.Delete, k.MoveLeft, k.MoveRight},
		{k.New, k.Assign, k.Cycle, k.Resume, k.Watch},
		{k.Project, k.MCP, k.Settings, k.Refresh, k.Help, k.Quit},
	}
}
