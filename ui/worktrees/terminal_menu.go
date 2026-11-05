package worktrees

import (
	"os"
	"path/filepath"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/terminalrun"
	"github.com/elentok/gx/ui/worktrees/launch"

	tea "charm.land/bubbletea/v2"
)

// moreOptionsInfo is shown when the terminal has no menu to offer.
const moreOptionsInfo = "use tmux, kitty, herdr or ghostty for more options"

// terminalMenuItems hides "session" on Ghostty, which has no session concept.
func terminalMenuItems(terminal ui.Terminal) []components.MenuItem {
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

// herdrTerminalDown reports a herdr terminal whose herdr is missing or
// unreachable: its splits and tabs would fail, so the menu stays hidden.
func (m Model) herdrTerminalDown() bool {
	return m.settings.Terminal == ui.TerminalHerdr && m.settings.HerdrUnavailable
}

// terminalMenu offers the ways to open a worktree in the terminal.
type terminalMenu struct {
	menu     components.MenuState
	name     string
	path     string
	title    string
	terminal ui.Terminal
	repoName string
	aliases  map[string]string
}

func (m Model) enterTerminalMenuFor(name, path string) Model {
	title := "Open in Terminal"
	if wt := m.cursorWorktree(); wt != nil {
		title = "Open: " + wt.Name
	}
	return m.openModal(terminalMenu{
		menu: components.MenuState{
			Items:  terminalMenuItems(m.settings.Terminal),
			Cursor: 0,
		},
		name:     name,
		path:     path,
		title:    title,
		terminal: m.settings.Terminal,
		repoName: filepath.Base(m.repo.LinkedWorktreeDir()),
		aliases:  m.settings.NameAliases,
	})
}

func (t terminalMenu) Update(msg tea.Msg) (modal, tea.Cmd, modalResult) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return t, nil, modalResult{unhandled: true}
	}
	newState, done, selected, handled := components.UpdateMenu(key, t.menu)
	t.menu = newState
	if !handled || !done {
		return t, nil, modalResult{}
	}
	if selected {
		item, _ := t.menu.Selected()
		return t, t.action(item.Value), modalResult{close: true}
	}
	return t, nil, modalResult{close: true}
}

func (t terminalMenu) InputFocused() bool { return true }

func (t terminalMenu) action(action string) tea.Cmd {
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
		return notify.Info(moreOptionsInfo)
	}
	return nil
}

func (t terminalMenu) View(_, _ int) string {
	return components.RenderMenuModal(
		t.title, "",
		t.menu,
		"",
		ui.ColorBorder, ui.ColorBlue, ui.ColorSubtle, ui.ColorText,
		40,
	)
}
