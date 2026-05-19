package log

import tea "charm.land/bubbletea/v2"

func (m Model) handlePushUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, result := m.push.Update(msg)
	m.push = next
	if result.Done {
		m.output.Set("Push output", result.Output)
		if result.Err != nil {
			m.statusMsg = "push failed: " + result.Err.Error()
		} else {
			m.statusMsg = "pushed"
		}
		return m, tea.Batch(cmd, m.cmdReload())
	}
	return m, cmd
}

func (m Model) handlePullUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, result := m.pull.Update(msg)
	m.pull = next
	if result.Done {
		m.output.Set("Pull output", result.Output)
		if result.Err != nil {
			m.statusMsg = "pull failed: " + result.Err.Error()
		} else {
			m.statusMsg = "pulled"
		}
		return m, m.cmdReload()
	}
	return m, cmd
}
