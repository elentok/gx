package commit

import (
	tea "charm.land/bubbletea/v2"
)

func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.help.IsOpen || m.amendConfirm.IsOpen || m.diffModel.Search().IsActive() || m.fileTreeModel.Search().IsActive() {
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
	if m.mouseOverDiff(mouse.X, mouse.Y) {
		m.diffModel.ScrollViewport(dir * 3)
	} else if m.mouseOverFiletree(mouse.X, mouse.Y) {
		m.fileTreeModel.SetVisibleHeight(m.filesInnerHeight())
		m.fileTreeModel.ScrollViewport(dir * 3)
	}
	return m, nil
}

func (m Model) mouseOverDiff(x, y int) bool {
	bodyH, contentH := m.layoutHeights()
	if y < bodyH || y >= bodyH+contentH {
		return false
	}
	if m.width < 90 {
		filesH, _ := m.narrowPaneHeights(contentH)
		return y >= bodyH+filesH
	}
	leftW := m.filesPaneWidth(contentH)
	return x >= leftW
}

func (m Model) mouseOverFiletree(x, y int) bool {
	bodyH, contentH := m.layoutHeights()
	if y < bodyH || y >= bodyH+contentH {
		return false
	}
	if m.width < 90 {
		filesH, _ := m.narrowPaneHeights(contentH)
		return y < bodyH+filesH
	}
	leftW := m.filesPaneWidth(contentH)
	return x < leftW
}
