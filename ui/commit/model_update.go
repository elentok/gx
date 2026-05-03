package commit

import (
	"github.com/elentok/gx/ui/explorer"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.syncDiffViewport()
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
			if m.focusDiff {
				m.focusDiff = false
				return m, nil
			}
			return m, nav.Back()
		case "b":
			m.bodyExpanded = !m.bodyExpanded
			m.syncDiffViewport()
			return m, nil
		case "a":
			if !m.focusDiff {
				return m, nil
			}
			if m.diffNavMode == explorer.NavHunk {
				m.diffNavMode = explorer.NavLine
			} else {
				m.diffNavMode = explorer.NavHunk
			}
			m.ensureActiveVisible()
			return m, nil
		case "w":
			if !m.focusDiff {
				return m, nil
			}
			m.wrapSoft = !m.wrapSoft
			m.syncDiffViewport()
			return m, nil
		case "j", "down":
			if m.focusDiff {
				m.moveDiffActive(1)
				return m, nil
			}
			if len(m.files) > 0 {
				if m.selected < len(m.files)-1 {
					m.selected++
					m.refreshDiff()
				}
			}
			return m, nil
		case "k", "up":
			if m.focusDiff {
				m.moveDiffActive(-1)
				return m, nil
			}
			if len(m.files) > 0 {
				if m.selected > 0 {
					m.selected--
					m.refreshDiff()
				}
			}
			return m, nil
		case "J":
			if m.focusDiff {
				m.diffViewport.ScrollDown(3)
			}
			return m, nil
		case "K":
			if m.focusDiff {
				m.diffViewport.ScrollUp(3)
			}
			return m, nil
		case "ctrl+d":
			if m.focusDiff {
				m.scrollDiffPage(1)
			}
			return m, nil
		case "ctrl+u":
			if m.focusDiff {
				m.scrollDiffPage(-1)
			}
			return m, nil
		case "G":
			if m.focusDiff {
				m.jumpDiffBottom()
				return m, nil
			}
		case "enter":
			if len(m.files) > 0 {
				m.focusDiff = true
				m.ensureActiveVisible()
			}
			return m, nil
		case "l", "right":
			if len(m.files) > 0 {
				m.focusDiff = true
				m.ensureActiveVisible()
			}
			return m, nil
		case "h", "left":
			if m.focusDiff {
				m.focusDiff = false
			}
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
