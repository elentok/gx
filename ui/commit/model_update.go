package commit

import (
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		return m, nil
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if handled, cmd := m.handleChordKey(msg); handled {
			return m, cmd
		}
		switch msg.String() {
		case "q", "esc":
			return m, nav.Back()
		case "b":
			m.bodyExpanded = !m.bodyExpanded
			return m, nil
		}
	}
	return m, nil
}

func (m Model) handleChordKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if m.keyPrefix == "g" {
		m.keyPrefix = ""
		switch msg.String() {
		case "w":
			return true, nav.Replace(nav.Route{Kind: nav.RouteWorktrees})
		case "l":
			return true, nav.Replace(nav.Route{Kind: nav.RouteLog, WorktreeRoot: m.worktreeRoot, Ref: m.ref})
		case "s":
			return true, nav.Replace(nav.Route{Kind: nav.RouteStatus, WorktreeRoot: m.worktreeRoot})
		}
		return true, nil
	}
	if msg.String() == "g" {
		m.keyPrefix = "g"
		return true, nil
	}
	return false, nil
}
