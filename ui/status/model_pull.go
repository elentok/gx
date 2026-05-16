package status

import tea "charm.land/bubbletea/v2"

func (m Model) handlePullUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, result := m.pull.Update(msg)
	m.pull = next
	if result.Done {
		m.output.Set("Pull output", result.Output)
		if result.Err != nil {
			m.setStatus("pull failed: " + result.Err.Error())
		} else {
			m.setStatus("pulled")
		}
		return m, m.refresh()
	}
	return m, cmd
}
