package worktrees

import (
	"github.com/elentok/gx/ui"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
)

type keyMap struct {
	Up           key.Binding
	Down         key.Binding
	Top          key.Binding
	New          key.Binding
	NewAndOpen   key.Binding
	Delete       key.Binding
	Rename       key.Binding
	Clone        key.Binding
	Yank         key.Binding
	Pull         key.Binding
	Push         key.Binding
	Rebase       key.Binding
	Search       key.Binding
	Track        key.Binding
	Refresh      key.Binding
	RemoteUpdate key.Binding
	GoOutput     key.Binding
	GoWorktrees  key.Binding
	GoLog        key.Binding
	GoStatus     key.Binding
	LazygitLog   key.Binding
	Open         key.Binding
	SearchNext   key.Binding
	SearchPrev   key.Binding
	SearchClose  key.Binding
	PasteConfirm key.Binding
	PasteCancel  key.Binding
	Help         key.Binding
	Quit         key.Binding
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
		key.WithKeys("gg"),
		key.WithHelp("gg", "top"),
	),
	Delete: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "delete"),
	),
	New: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "new worktree"),
	),
	NewAndOpen: key.NewBinding(
		key.WithKeys("N"),
		key.WithHelp("N", "new worktree + open"),
	),
	Open: key.NewBinding(
		key.WithKeys("o"),
		key.WithHelp("o", "open in terminal"),
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
	GoOutput: key.NewBinding(
		key.WithKeys("go"),
		key.WithHelp("go", "view output"),
	),
	GoWorktrees: key.NewBinding(
		key.WithKeys("gw"),
		key.WithHelp("gw", "goto worktrees"),
	),
	GoLog: key.NewBinding(
		key.WithKeys("gl"),
		key.WithHelp("gl", "goto log"),
	),
	GoStatus: key.NewBinding(
		key.WithKeys("gs"),
		key.WithHelp("gs", "goto status"),
	),
	LazygitLog: key.NewBinding(
		key.WithKeys("L"),
		key.WithHelp("L", "lazygit log"),
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
		key.WithHelp("?", "help"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Top, k.New, k.NewAndOpen, k.Open, k.Delete, k.Rename, k.Clone, k.Yank, k.Pull, k.Push, k.Search, k.Track, k.Refresh, k.Quit, k.Help}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top},
		{k.New, k.NewAndOpen, k.Open, k.Delete, k.Rename, k.Clone},
		{k.Yank, k.Search},
		{k.Pull, k.Push, k.Rebase, k.Track, k.Refresh, k.RemoteUpdate, k.GoWorktrees, k.GoLog, k.GoStatus, k.LazygitLog, k.Help, k.Quit},
	}
}

func newWorktreeHelpModel() help.Model {
	h := help.New()
	h.ShortSeparator = " · "
	h.FullSeparator = "  "
	h.Styles.ShortKey = h.Styles.ShortKey.Foreground(ui.ColorBlue).Bold(true)
	h.Styles.ShortDesc = h.Styles.ShortDesc.Foreground(ui.ColorSubtle)
	h.Styles.ShortSeparator = h.Styles.ShortSeparator.Foreground(ui.ColorSubtle)
	h.Styles.FullKey = h.Styles.FullKey.Foreground(ui.ColorBlue).Bold(true)
	h.Styles.FullDesc = h.Styles.FullDesc.Foreground(ui.ColorText)
	h.Styles.FullSeparator = h.Styles.FullSeparator.Foreground(ui.ColorSubtle)
	return h
}
