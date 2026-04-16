package worktrees

import "gx/ui"

import tea "charm.land/bubbletea/v2"

func (m Model) handleOutputKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	key := msg.String()
	if m.keyPrefix == "o" {
		m.keyPrefix = ""
		switch key {
		case "o":
			if m.lastJobLog == "" {
				m.statusGen++
				m.statusMsg = ui.MessageNoOutput()
				return m, cmdClearStatus(m.statusGen), true
			}
			return m.enterLogsMode(), nil, true
		case "l":
			wt := m.selectedWorktree()
			if wt != nil {
				m.statusMsg = ui.MessageOpening("lazygit log")
				return m, cmdLazygitLog(*wt), true
			}
			return m, nil, true
		case "t":
			wt := m.selectedWorktree()
			if wt != nil {
				m.statusMsg = ui.MessageOpening("tmux session")
				return m, cmdTmuxNewSession(wt.Name, wt.Path), true
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
		if len(m.worktrees) == 0 {
			return m, nil, true
		}
		m.table.SetCursor(0)
		m.statusMsg = ""
		return m, cmdLoadSidebarData(m.repo, m.worktrees[0]), true
	}
	if key == "o" {
		m.keyPrefix = "o"
		m.statusMsg = ui.RenderInlineBindings(keys.Logs, keys.Log, keys.TmuxSession)
		return m, nil, true
	}
	return m, nil, false
}
