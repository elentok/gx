package modal

import (
	"os"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/terminalrun"
	"github.com/elentok/gx/ui/worktrees/launch"

	tea "charm.land/bubbletea/v2"
)

// MoreOptionsInfo is shown when the terminal has no menu to offer.
const MoreOptionsInfo = "use tmux, kitty, herdr or ghostty for more options"

// TerminalMenuItems hides "session" on Ghostty, which has no session concept.
func TerminalMenuItems(terminal ui.Terminal) []components.MenuItem {
	items := []components.MenuItem{
		{Label: "hsplit", Value: "hsplit", Detail: "horizontal split (stacked, top/bottom)"},
		{Label: "vsplit", Value: "vsplit", Detail: "vertical split (side-by-side)"},
		{Label: "tab", Value: "tab", Detail: "new tab"},
	}
	if terminal == ui.TerminalGhostty {
		return items
	}
	session := components.MenuItem{Label: "session", Value: "session", Detail: "new or jump to existing"}
	return append([]components.MenuItem{session}, items...)
}

// TerminalMenu offers the ways to open a worktree in the terminal.
type TerminalMenu struct {
	menu     components.MenuState
	name     string
	path     string
	title    string
	terminal ui.Terminal
	repoName string
	aliases  map[string]string
}

// NewTerminalMenu offers the ways to open the worktree name at path in
// terminal.
func NewTerminalMenu(terminal ui.Terminal, name, path, title, repoName string, aliases map[string]string) TerminalMenu {
	return TerminalMenu{
		menu: components.MenuState{
			Items:  TerminalMenuItems(terminal),
			Cursor: 0,
		},
		name:     name,
		path:     path,
		title:    title,
		terminal: terminal,
		repoName: repoName,
		aliases:  aliases,
	}
}

// Menu is the menu's current state.
func (t TerminalMenu) Menu() components.MenuState { return t.menu }

func (t TerminalMenu) Update(msg tea.Msg) (Modal, tea.Cmd, Result) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return t, nil, Result{Unhandled: true}
	}
	newState, done, selected, handled := components.UpdateMenu(key, t.menu)
	t.menu = newState
	if !handled || !done {
		return t, nil, Result{}
	}
	if selected {
		item, _ := t.menu.Selected()
		return t, t.Action(item.Value), Result{Close: true}
	}
	return t, nil, Result{Close: true}
}

func (t TerminalMenu) InputFocused() bool { return true }

// Action is the command for one menu choice.
func (t TerminalMenu) Action(action string) tea.Cmd {
	name, path := t.name, t.path

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	done := func(err error, _ string) tea.Msg { return launch.TerminalResultMsg{Err: err} }

	switch t.terminal {
	case ui.TerminalTmux:
		switch action {
		case "session":
			return launch.TmuxNewSession(name, path)
		case "hsplit":
			return terminalrun.CommandWithSplitShell(path, ui.TerminalTmux, terminalrun.HSplit, shell, done)
		case "vsplit":
			return terminalrun.CommandWithSplitShell(path, ui.TerminalTmux, terminalrun.VSplit, shell, done)
		case "tab":
			return terminalrun.CommandWithSplitShell(path, ui.TerminalTmux, terminalrun.Tab, shell, done)
		}
	case ui.TerminalKittyRemote:
		sessName := launch.SessionNameFor(t.repoName, name, t.aliases)
		switch action {
		case "session":
			return launch.KittySession(sessName, path)
		case "hsplit":
			return terminalrun.CommandWithSplitShell(path, ui.TerminalKittyRemote, terminalrun.HSplit, shell, done)
		case "vsplit":
			return terminalrun.CommandWithSplitShell(path, ui.TerminalKittyRemote, terminalrun.VSplit, shell, done)
		case "tab":
			return terminalrun.CommandWithSplitShell(path, ui.TerminalKittyRemote, terminalrun.Tab, shell, done)
		}
	case ui.TerminalHerdr:
		sessName := launch.SessionNameFor(t.repoName, name, t.aliases)
		switch action {
		case "session":
			return launch.HerdrSession(sessName, path)
		case "hsplit":
			return terminalrun.CommandWithSplitShell(path, ui.TerminalHerdr, terminalrun.HSplit, shell, done)
		case "vsplit":
			return terminalrun.CommandWithSplitShell(path, ui.TerminalHerdr, terminalrun.VSplit, shell, done)
		case "tab":
			return terminalrun.CommandWithSplitShell(path, ui.TerminalHerdr, terminalrun.Tab, shell, done)
		}
	case ui.TerminalGhostty:
		switch action {
		case "hsplit":
			return terminalrun.CommandWithSplitShell(path, ui.TerminalGhostty, terminalrun.HSplit, shell, done)
		case "vsplit":
			return terminalrun.CommandWithSplitShell(path, ui.TerminalGhostty, terminalrun.VSplit, shell, done)
		case "tab":
			return terminalrun.CommandWithSplitShell(path, ui.TerminalGhostty, terminalrun.Tab, shell, done)
		}
	case ui.TerminalKitty:
		return notify.Info("enable kitty remote control for this to work")
	default:
		return notify.Info(MoreOptionsInfo)
	}
	return nil
}

func (t TerminalMenu) View(_, _ int) string {
	return components.RenderMenuModal(
		t.title, "",
		t.menu,
		"",
		ui.ColorBorder, ui.ColorBlue, ui.ColorSubtle, ui.ColorText,
		40,
	)
}
