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
	if m.commitOps.AmendOpen() || m.diff.diffModel.Search().IsActive() || m.sidebar.fileTreeModel.Search().IsActive() {
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
	l := m.layout()
	idx, ok := ui.HoverHitTest(mouse.X, mouse.Y, l.diffRect(), l.filetreeRect())
	if !ok {
		return m, nil
	}
	if idx == 0 {
		m.diff.diffModel.ScrollViewport(dir * 3)
	} else {
		m.sidebar.fileTreeModel.SetVisibleHeight(l.filesInnerHeight())
		m.sidebar.fileTreeModel.ScrollViewport(dir * 3)
	}
	return m, nil
}
