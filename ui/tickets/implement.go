package tickets

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

func (m Model) handleConfirmUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, _ := m.confirm.Update(msg)
	m.confirm = next
	return m, cmd
}

func (m Model) handleConfirmMouseUpdate(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	next, cmd, _ := m.confirm.UpdateMouse(msg, m.width, m.width, m.height)
	m.confirm = next
	return m, cmd
}

func (m Model) handleImplementSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if len(m.implementingEpics) == 0 {
		return m, nil
	}
	var cmd tea.Cmd
	m.implementSpinner, cmd = m.implementSpinner.Update(msg)
	return m, cmd
}
