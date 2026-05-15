package log

import (
	tea "charm.land/bubbletea/v2"
)

func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.help.IsOpen || m.amendConfirm.IsOpen {
		return m, nil
	}
	mouse := msg.Mouse()
	dir := 0
	switch mouse.Button {
	case tea.MouseWheelDown:
		dir = 1
	case tea.MouseWheelUp:
		dir = -1
	default:
		return m, nil
	}
	m.list.ScrollViewport(dir*3, len(m.rows), maxInt(1, m.height-3))
	return m, nil
}
