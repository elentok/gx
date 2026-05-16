package status

import tea "charm.land/bubbletea/v2"

func (m Model) handlePushUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, result := m.push.Update(msg)
	m.push = next
	if result.Done {
		m.output.Set("Push output", result.Output)
		if result.Err != nil {
			m.setStatus("push failed: " + result.Err.Error())
		} else {
			m.setStatus("pushed")
		}
		return m, m.refresh()
	}
	return m, cmd
}
