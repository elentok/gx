package worktrees

import (
	"os"
	"path/filepath"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/terminalrun"

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

func (m Model) enterTerminalMenuFor(name, path string) Model {
	m.mode = modeTerminalMenu
	m.openTargetName = name
	m.openTargetPath = path
	m.terminalMenu = components.MenuState{
		Items:  terminalMenuItems(m.settings.Terminal),
		Cursor: 0,
	}
	return m
}

func (m Model) handleTerminalMenuKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	newState, done, selected, handled := components.UpdateMenu(msg, m.terminalMenu)
	m.terminalMenu = newState
	if !handled || !done {
		return m, nil
	}
	if selected {
		item, _ := m.terminalMenu.Selected()
		return m.executeTerminalAction(item.Value)
	}
	m.mode = modeNormal
	return m, nil
}

func (m Model) executeTerminalAction(action string) (Model, tea.Cmd) {
	m.mode = modeNormal
	name := m.openTargetName
	path := m.openTargetPath

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	done := func(err error, _ string) tea.Msg { return terminalResultMsg{err: err} }

	switch m.settings.Terminal {
	case ui.TerminalTmux:
		switch action {
		case "session":
			return m, cmdTmuxNewSession(name, path)
		case "hsplit":
			return m, terminalrun.CommandWithSplitShell(path, ui.TerminalTmux, terminalrun.HSplit, shell, done)
		case "vsplit":
			return m, terminalrun.CommandWithSplitShell(path, ui.TerminalTmux, terminalrun.VSplit, shell, done)
		case "tab":
			return m, terminalrun.CommandWithSplitShell(path, ui.TerminalTmux, terminalrun.Tab, shell, done)
		}
	case ui.TerminalKittyRemote:
		repoName := filepath.Base(m.repo.LinkedWorktreeDir())
		sessName := sessionNameFor(repoName, name, m.settings.NameAliases)
		switch action {
		case "session":
			return m, cmdKittySession(sessName, path)
		case "hsplit":
			return m, terminalrun.CommandWithSplitShell(path, ui.TerminalKittyRemote, terminalrun.HSplit, shell, done)
		case "vsplit":
			return m, terminalrun.CommandWithSplitShell(path, ui.TerminalKittyRemote, terminalrun.VSplit, shell, done)
		case "tab":
			return m, terminalrun.CommandWithSplitShell(path, ui.TerminalKittyRemote, terminalrun.Tab, shell, done)
		}
	case ui.TerminalHerdr:
		repoName := filepath.Base(m.repo.LinkedWorktreeDir())
		sessName := sessionNameFor(repoName, name, m.settings.NameAliases)
		switch action {
		case "session":
			return m, cmdHerdrSession(sessName, path)
		case "hsplit":
			return m, terminalrun.CommandWithSplitShell(path, ui.TerminalHerdr, terminalrun.HSplit, shell, done)
		case "vsplit":
			return m, terminalrun.CommandWithSplitShell(path, ui.TerminalHerdr, terminalrun.VSplit, shell, done)
		case "tab":
			return m, terminalrun.CommandWithSplitShell(path, ui.TerminalHerdr, terminalrun.Tab, shell, done)
		}
	case ui.TerminalGhostty:
		switch action {
		case "hsplit":
			return m, terminalrun.CommandWithSplitShell(path, ui.TerminalGhostty, terminalrun.HSplit, shell, done)
		case "vsplit":
			return m, terminalrun.CommandWithSplitShell(path, ui.TerminalGhostty, terminalrun.VSplit, shell, done)
		case "tab":
			return m, terminalrun.CommandWithSplitShell(path, ui.TerminalGhostty, terminalrun.Tab, shell, done)
		}
	case ui.TerminalKitty:
		return m, notify.Info("enable kitty remote control for this to work")
	default:
		return m, notify.Info(moreOptionsInfo)
	}
	return m, nil
}

func (m Model) terminalMenuModalView() string {
	wt := m.cursorWorktree()
	title := "Open in Terminal"
	if wt != nil {
		title = "Open: " + wt.Name
	}
	return components.RenderMenuModal(
		title, "",
		m.terminalMenu,
		"",
		ui.ColorBorder, ui.ColorBlue, ui.ColorSubtle, ui.ColorText,
		40,
	)
}
