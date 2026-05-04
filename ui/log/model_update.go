package log

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

		m.help, cmd = m.help.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.help.IsOpen {
			m.help, cmd = m.help.Update(msg)
			return m, cmd
		}
		if m.searchMode == searchModeInput {
			return m.handleSearchKey(msg)
		}
		if handled, cmd := m.handleChordKey(msg); handled {
			return m, cmd
		}
		if handled := m.handleTagJumpChord(msg); handled {
			return m, nil
		}
		switch msg.String() {
		case "?":
			m.help.Open(m.width, m.height)
			return m, nil
		case "q":
			if m.settings.EnableNavigation {
				return m, nav.Back()
			}
			return m, tea.Quit
		case "esc":
			if m.settings.EnableNavigation {
				return m, nav.Back()
			}
			return m, nil
		case "j", "down":
			if m.cursor < len(m.rows)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "g":
			m.keyPrefix = "g"
		case "]", "[":
			m.keyPrefix = msg.String()
		case "G":
			if len(m.rows) > 0 {
				m.cursor = len(m.rows) - 1
			}
		case "/":
			m.enterSearchMode()
		case "n":
			m.advanceSearch(1)
		case "N":
			m.advanceSearch(-1)
		case "enter":
			return m, m.openSelected()
		case "R":
			m.reload()
		}
	}
	return m, nil
}

func (m *Model) handleTagJumpChord(msg tea.KeyPressMsg) bool {
	if m.keyPrefix != "]" && m.keyPrefix != "[" {
		return false
	}
	prefix := m.keyPrefix
	m.keyPrefix = ""
	if msg.String() != "t" {
		return true
	}
	step := 1
	if prefix == "[" {
		step = -1
	}
	m.jumpToTaggedCommit(step)
	return true
}

func (m *Model) jumpToTaggedCommit(step int) {
	if len(m.rows) == 0 || step == 0 {
		return
	}
	for i := m.cursor + step; i >= 0 && i < len(m.rows); i += step {
		if rowHasTag(m.rows[i]) {
			m.cursor = i
			return
		}
	}
}

func rowHasTag(r row) bool {
	for _, decoration := range r.commit.Decorations {
		if decoration.Kind == git.RefDecorationTag {
			return true
		}
	}
	return false
}

func (m *Model) handleChordKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if m.keyPrefix != "g" {
		return false, nil
	}
	m.keyPrefix = ""
	switch msg.String() {
	case "g":
		m.cursor = 0
		m.statusMsg = ""
		return true, nil
	case "w":
		m.statusMsg = ""
		return true, nav.Replace(nav.Route{Kind: nav.RouteWorktrees})
	case "s":
		m.statusMsg = ""
		return true, nav.Replace(nav.Route{Kind: nav.RouteStatus, WorktreeRoot: m.worktreeRoot})
	case "l":
		m.statusMsg = ""
		return true, nav.Replace(nav.Route{Kind: nav.RouteLog, WorktreeRoot: m.worktreeRoot, Ref: m.startRef})
	case "h":
		m.statusMsg = ""
		if m.startRef != "HEAD" {
			return true, nav.Replace(nav.Route{Kind: nav.RouteLog, WorktreeRoot: m.worktreeRoot, Ref: "HEAD"})
		}
		return true, nil
	case "esc":
		m.statusMsg = ""
		return true, nil
	default:
		m.statusMsg = ""
		return true, nil
	}
}
