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

func terminalMenuItems() []components.MenuItem {
	return []components.MenuItem{
		{Label: "s  session", Value: "session", Detail: "new or jump to existing"},
		{Label: "h  hsplit", Value: "hsplit", Detail: "horizontal split (stacked, top/bottom)"},
		{Label: "v  vsplit", Value: "vsplit", Detail: "vertical split (side-by-side)"},
		{Label: "t  tab", Value: "tab", Detail: "new tab"},
	}
}

func (m Model) enterTerminalMenuFor(name, path string) Model {
	m.mode = modeTerminalMenu
	m.openTargetName = name
	m.openTargetPath = path
	m.terminalMenu = components.MenuState{
		Items:  terminalMenuItems(),
		Cursor: 0,
	}
	return m
}

func (m Model) handleTerminalMenuKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "s":
		return m.executeTerminalAction("session")
	case "h":
		return m.executeTerminalAction("hsplit")
	case "v":
		return m.executeTerminalAction("vsplit")
	case "t":
		return m.executeTerminalAction("tab")
	default:
		newState, done, selected, handled := components.UpdateMenu(msg, m.terminalMenu)
		m.terminalMenu = newState
		if !handled {
			return m, nil
		}
		if done {
			if selected {
				return m.executeTerminalAction(m.terminalMenu.Items[m.terminalMenu.Cursor].Value)
			}
			m.mode = modeNormal
		}
		return m, nil
	}
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
			return m, terminalrun.CommandWithSplitBare(path, ui.TerminalTmux, terminalrun.HSplit, shell, nil, done)
		case "vsplit":
			return m, terminalrun.CommandWithSplitBare(path, ui.TerminalTmux, terminalrun.VSplit, shell, nil, done)
		case "tab":
			return m, terminalrun.CommandWithSplitBare(path, ui.TerminalTmux, terminalrun.Tab, shell, nil, done)
		}
	case ui.TerminalKittyRemote:
		repoName := filepath.Base(m.repo.LinkedWorktreeDir())
		sessName := sessionNameFor(repoName, name, m.settings.NameAliases)
		switch action {
		case "session":
			return m, cmdKittySession(sessName, path)
		case "hsplit":
			return m, terminalrun.CommandWithSplitBare(path, ui.TerminalKittyRemote, terminalrun.HSplit, shell, nil, done)
		case "vsplit":
			return m, terminalrun.CommandWithSplitBare(path, ui.TerminalKittyRemote, terminalrun.VSplit, shell, nil, done)
		case "tab":
			return m, terminalrun.CommandWithSplitBare(path, ui.TerminalKittyRemote, terminalrun.Tab, shell, nil, done)
		}
	case ui.TerminalKitty:
		return m, notify.Info("enable kitty remote control for this to work")
	default:
		return m, notify.Info("use tmux or kitty for more options")
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
