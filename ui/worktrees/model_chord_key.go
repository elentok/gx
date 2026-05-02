package worktrees

import (
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

// handleChordKey handles keys that form chords (g-prefix) or open the terminal menu (o).
// Returns (model, cmd, handled).
func (m Model) handleChordKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	key := msg.String()
	if m.keyPrefix == "g" {
		m.keyPrefix = ""
		switch key {
		case "g":
			if len(m.worktrees) == 0 {
				return m, nil, true
			}
			m.table.SetCursor(0)
			m.statusMsg = ""
			return m, cmdLoadSidebarData(m.repo, m.worktrees[0]), true
		case "o":
			if m.lastJobLog == "" {
				m.statusGen++
				m.statusMsg = ui.MessageNoOutput()
				return m, cmdClearStatus(m.statusGen), true
			}
			return m.enterLogsMode(), nil, true
		case "l":
			if m.settings.EnableNavigation {
				wt := m.selectedWorktree()
				if wt != nil {
					m.statusMsg = ""
					return m, nav.Push(nav.Route{Kind: nav.RouteLog, WorktreeRoot: wt.Path}), true
				}
				return m, nil, true
			}
			return m, nil, true
		case "s":
			if m.settings.EnableNavigation {
				wt := m.selectedWorktree()
				if wt != nil {
					m.statusMsg = ""
					return m, nav.Push(nav.Route{Kind: nav.RouteStatus, WorktreeRoot: wt.Path}), true
				}
				return m, nil, true
			}
			return m, nil, true
		case "w":
			if m.settings.EnableNavigation {
				m.statusMsg = ""
				return m, nav.Push(nav.Route{Kind: nav.RouteWorktrees}), true
			}
			return m, nil, true
		case "esc":
			m.statusMsg = ""
			return m, nil, true
		default:
			m.statusMsg = ""
			return m, nil, true
		}
	}
	if key == "g" {
		m.keyPrefix = "g"
		m.statusMsg = ui.RenderInlineBindings(keys.Top, keys.GoOutput, keys.GoWorktrees, keys.GoLog, keys.GoStatus)
		return m, nil, true
	}
	if key == "L" {
		wt := m.selectedWorktree()
		if wt != nil {
			m.statusMsg = ui.MessageOpening("lazygit log")
			return m, cmdLazygitLog(*wt), true
		}
		return m, nil, true
	}
	if key == "o" {
		if m.settings.Terminal == ui.TerminalPlain {
			m.statusGen++
			m.statusMsg = "use tmux or kitty for more options"
			return m, cmdClearStatus(m.statusGen), true
		}
		wt := m.selectedWorktree()
		if wt != nil {
			return m.enterTerminalMenuFor(wt.Name, wt.Path), nil, true
		}
		return m, nil, true
	}
	return m, nil, false
}
