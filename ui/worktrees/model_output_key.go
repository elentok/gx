package worktrees

import tea "charm.land/bubbletea/v2"

func (m Model) handleOutputKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	key := msg.String()
	if m.keyPrefix == "o" {
		m.keyPrefix = ""
		switch key {
		case "o":
			if m.lastJobLog == "" {
				m.statusGen++
				m.statusMsg = "no command output"
				return m, cmdClearStatus(m.statusGen), true
			}
			return m.enterLogsMode(), nil, true
		case "l":
			wt := m.selectedWorktree()
			if wt != nil {
				m.statusMsg = "opening lazygit log..."
				return m, cmdLazygitLog(*wt), true
			}
			return m, nil, true
		case "t":
			wt := m.selectedWorktree()
			if wt != nil {
				m.statusMsg = "opening tmux session..."
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
		m.statusMsg = "oo: output · ol: lazygit log · ot: tmux session"
		return m, nil, true
	}
	return m, nil, false
}
