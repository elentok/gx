package commit

import (
	tea "charm.land/bubbletea/v2"
)

func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.help.IsOpen || m.amendConfirm.IsOpen || m.search.IsActive() {
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
	if !m.focusDiff && !m.mouseOverDiff(mouse.X, mouse.Y) {
		return m, nil
	}
	if dir > 0 {
		m.diffModel.Viewport().ScrollDown(3)
	} else {
		m.diffModel.Viewport().ScrollUp(3)
	}
	return m, nil
}

func (m Model) mouseOverDiff(x, y int) bool {
	bodyH, contentH := m.layoutHeights()
	if y < bodyH || y >= bodyH+contentH {
		return false
	}
	if m.width < 90 {
		filesH := max(5, contentH/3)
		return y >= bodyH+filesH
	}
	leftW := m.filesPaneWidth(contentH)
	return x >= leftW
}
