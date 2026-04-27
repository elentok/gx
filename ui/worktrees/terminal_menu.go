package worktrees

import (
	"path/filepath"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"

	tea "charm.land/bubbletea/v2"
)

func terminalMenuItems() []components.MenuItem {
	return []components.MenuItem{
		{Label: "s  session", Value: "session", Detail: "new or jump to existing"},
		{Label: "h  hsplit", Value: "hsplit", Detail: "horizontal split"},
		{Label: "v  vsplit", Value: "vsplit", Detail: "vertical split"},
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
			m.statusMsg = ""
		}
		return m, nil
	}
}

func (m Model) executeTerminalAction(action string) (Model, tea.Cmd) {
	m.mode = modeNormal
	name := m.openTargetName
	path := m.openTargetPath

	switch m.settings.Terminal {
	case ui.TerminalTmux:
		switch action {
		case "session":
			return m, cmdTmuxNewSession(name, path)
		case "hsplit":
			return m, cmdTmuxHSplit(path)
		case "vsplit":
			return m, cmdTmuxVSplit(path)
		case "tab":
			return m, cmdTmuxNewWindow(name, path)
		}
	case ui.TerminalKittyRemote:
		repoName := filepath.Base(m.repo.LinkedWorktreeDir())
		sessName := sessionNameFor(repoName, name)
		switch action {
		case "session":
			return m, cmdKittySession(sessName, path)
		case "hsplit":
			return m, cmdKittySplit(path, true)
		case "vsplit":
			return m, cmdKittySplit(path, false)
		case "tab":
			return m, cmdKittyNewTab(path)
		}
	case ui.TerminalKitty:
		m.statusGen++
		m.statusMsg = "enable kitty remote control for this to work"
		return m, cmdClearStatus(m.statusGen)
	default:
		m.statusGen++
		m.statusMsg = "use tmux or kitty for more options"
		return m, cmdClearStatus(m.statusGen)
	}
	return m, nil
}

func (m Model) terminalMenuModalView() string {
	wt := m.selectedWorktree()
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
