package worktrees

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Up             key.Binding
	Down           key.Binding
	Top            key.Binding
	New            key.Binding
	NewTmuxSession key.Binding
	NewTmuxWindow  key.Binding
	Delete         key.Binding
	Rename         key.Binding
	Clone          key.Binding
	Yank           key.Binding
	Pull           key.Binding
	Push           key.Binding
	Rebase         key.Binding
	Search         key.Binding
	Track          key.Binding
	Refresh        key.Binding
	RemoteUpdate   key.Binding
	Logs           key.Binding
	Log            key.Binding
	TmuxSession    key.Binding
	SearchNext     key.Binding
	SearchPrev     key.Binding
	SearchClose    key.Binding
	PasteConfirm   key.Binding
	PasteCancel    key.Binding
	Help           key.Binding
	Quit           key.Binding
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Top: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "top"),
	),
	Delete: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "delete"),
	),
	New: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "new worktree"),
	),
	NewTmuxSession: key.NewBinding(
		key.WithKeys("N"),
		key.WithHelp("N", "new worktree + tmux session"),
	),
	NewTmuxWindow: key.NewBinding(
		key.WithKeys("T"),
		key.WithHelp("T", "new worktree + tmux window"),
	),
	Rename: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "rename"),
	),
	Clone: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "clone"),
	),
	Yank: key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "yank files"),
	),
	Pull: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "pull"),
	),
	Rebase: key.NewBinding(
		key.WithKeys("b"),
		key.WithHelp("b", "rebase on main"),
	),
	Push: key.NewBinding(
		key.WithKeys("P"),
		key.WithHelp("P", "push"),
	),
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
	Track: key.NewBinding(
		key.WithKeys("t"),
		key.WithHelp("t", "track"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("R"),
		key.WithHelp("R", "refresh"),
	),
	RemoteUpdate: key.NewBinding(
		key.WithKeys("U"),
		key.WithHelp("U", "remote update"),
	),
	Logs: key.NewBinding(
		key.WithKeys("oo"),
		key.WithHelp("oo", "view output"),
	),
	Log: key.NewBinding(
		key.WithKeys("ol"),
		key.WithHelp("ol", "lazygit log"),
	),
	TmuxSession: key.NewBinding(
		key.WithKeys("ot"),
		key.WithHelp("ot", "tmux session"),
	),
	SearchNext: key.NewBinding(
		key.WithKeys("ctrl+n"),
		key.WithHelp("ctrl+n", "next"),
	),
	SearchPrev: key.NewBinding(
		key.WithKeys("ctrl+p"),
		key.WithHelp("ctrl+p", "prev"),
	),
	SearchClose: key.NewBinding(
		key.WithKeys("esc", "enter"),
		key.WithHelp("esc/enter", "close"),
	),
	PasteConfirm: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "paste"),
	),
	PasteCancel: key.NewBinding(
		key.WithKeys("esc", "q"),
		key.WithHelp("esc/q", "cancel"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "toggle help"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}

// ShortHelp and FullHelp implement help.KeyMap so this can be wired to
// bubbles/help in a later milestone.

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Top, k.New, k.NewTmuxSession, k.NewTmuxWindow, k.Delete, k.Rename, k.Clone, k.Yank, k.Pull, k.Push, k.Search, k.Track, k.Refresh, k.Quit, k.Help}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top},
		{k.New, k.NewTmuxSession, k.NewTmuxWindow, k.Delete, k.Rename, k.Clone},
		{k.Yank, k.Search},
		{k.Pull, k.Push, k.Rebase, k.Track, k.Refresh, k.RemoteUpdate, k.Logs, k.Log, k.TmuxSession, k.Help, k.Quit},
	}
}
