package status

import (
	"github.com/elentok/gx/ui"

	tea "charm.land/bubbletea/v2"
)

func handleMouseWheel(m *Model, msg tea.MouseWheelMsg) bool {
	if m.modals.RunningOpen() || m.modals.ConfirmOpen() || m.modals.ErrorOpen() || m.help.IsOpen || searchActive(*m) {
		return false
	}
	mouse := msg.Mouse()
	dir := 0
	switch mouse.Button {
	case tea.MouseWheelDown:
		dir = 1
	case tea.MouseWheelUp:
		dir = -1
	default:
		return false
	}
	if scrollDiffByMouse(m, mouse.X, mouse.Y, dir) {
		return true
	}
	return scrollFiletreeByMouse(m, mouse.X, mouse.Y, dir)
}

func searchActive(m Model) bool {
	return m.filetree.Model.Search().IsActive() ||
		m.diffarea.Unstaged.Search().IsActive() ||
		m.diffarea.Staged.Search().IsActive()
}

func scrollDiffByMouse(m *Model, x, y, dir int) bool {
	if m.height < 1 {
		return false
	}

	diffX, diffY, diffW, diffH, ok := m.diffRect(mainContentHeight(m.height))
	if !ok || !(ui.Rect{X: diffX, Y: diffY, W: diffW, H: diffH}).Contains(x, y) {
		return false
	}

	diffviewModel := m.diffarea.SectionAt(y-diffY, diffH)
	if diffviewModel == nil {
		return false
	}
	diffviewModel.ScrollViewport(dir * 3)
	return true
}

func scrollFiletreeByMouse(m *Model, x, y, dir int) bool {
	if m.height < 1 {
		return false
	}
	filetreeX, filetreeY, filetreeW, filetreeH := m.filetreeRect(mainContentHeight(m.height))
	if !(ui.Rect{X: filetreeX, Y: filetreeY, W: filetreeW, H: filetreeH}).Contains(x, y) {
		return false
	}
	m.filetree.Model.SetVisibleHeight(m.filetree.InnerHeight(filetreeH, filetreeViewOf(*m)))
	m.filetree.Model.ScrollViewport(dir * 3)
	return true
}

func (m Model) filetreeRect(mainH int) (x, y, w, h int) {
	if useStackedLayout(m.width) {
		filetreeH, _ := splitHeight(m.width, mainH)
		return 0, 0, m.width, filetreeH
	}
	filetreeW, _ := m.splitWidth()
	return 0, 0, filetreeW, mainH
}

func (m Model) diffRect(mainH int) (x, y, w, h int, ok bool) {
	if m.diffarea.Fullscreen && m.focus == focusDiff {
		return 0, 0, m.width, mainH, true
	}
	if useStackedLayout(m.width) {
		filetreeH, diffH := splitHeight(m.width, mainH)
		return 0, filetreeH, m.width, diffH, true
	}
	filetreeW, diffW := m.splitWidth()
	return filetreeW, 0, diffW, mainH, true
}
