package worktrees

import tea "charm.land/bubbletea/v2"

func (m Model) handleOutputKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	key := msg.String()
	if key == "o" {
		if m.lastJobLog == "" {
			m.statusGen++
			m.statusMsg = "no command output"
			return m, cmdClearStatus(m.statusGen), true
		}
		return m.enterLogsMode(), nil, true
	}
	return m, nil, false
}
