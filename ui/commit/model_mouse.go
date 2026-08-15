package commit

import (
	"github.com/elentok/gx/ui"

	tea "charm.land/bubbletea/v2"
)

func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.help.Forward(msg); handled {
		m.help = next
		return m, cmd
	}
	if m.amendConfirm.IsOpen || m.diffModel.Search().IsActive() || m.fileTreeModel.Search().IsActive() {
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
	idx, ok := ui.HoverHitTest(mouse.X, mouse.Y, m.diffRect(), m.filetreeRect())
	if !ok {
		return m, nil
	}
	if idx == 0 {
		m.diffModel.ScrollViewport(dir * 3)
	} else {
		m.fileTreeModel.SetVisibleHeight(m.filesInnerHeight())
		m.fileTreeModel.ScrollViewport(dir * 3)
	}
	return m, nil
}

func (m Model) diffRect() ui.Rect {
	bodyH, contentH := m.layoutHeights()
	if m.width < 90 {
		filesH, diffH := m.narrowPaneHeights(contentH)
		return ui.Rect{X: 0, Y: bodyH + filesH, W: m.width, H: diffH}
	}
	leftW := m.filesPaneWidth(contentH)
	return ui.Rect{X: leftW, Y: bodyH, W: m.width - leftW, H: contentH}
}

func (m Model) filetreeRect() ui.Rect {
	bodyH, contentH := m.layoutHeights()
	if m.width < 90 {
		filesH, _ := m.narrowPaneHeights(contentH)
		return ui.Rect{X: 0, Y: bodyH, W: m.width, H: filesH}
	}
	leftW := m.filesPaneWidth(contentH)
	return ui.Rect{X: 0, Y: bodyH, W: leftW, H: contentH}
}
