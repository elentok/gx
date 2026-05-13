package status

import (
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/status/diffarea"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) handleMouseWheel(msg tea.MouseWheelMsg) bool {
	if m.runningOpen || m.confirmOpen || m.errorOpen || m.help.IsOpen || m.searchActive() {
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
	return m.scrollDiffByMouse(mouse.X, mouse.Y, dir)
}

func (m Model) searchActive() bool {
	return m.fileTreeModel.Search().IsActive() ||
		m.diffarea.Unstaged.Search().IsActive() ||
		m.diffarea.Staged.Search().IsActive()
}

func (m *Model) scrollDiffByMouse(x, y, dir int) bool {
	mainH := m.height - 1
	if mainH < 1 || y < 0 || y >= mainH || x < 0 || x >= m.width {
		return false
	}

	diffX, diffY, diffW, diffH, ok := m.diffRect(mainH)
	if !ok || x < diffX || x >= diffX+diffW || y < diffY || y >= diffY+diffH {
		return false
	}

	diffviewModel := m.mouseTargetSection(y-diffY, diffH)
	if diffviewModel == nil {
		return false
	}
	if dir > 0 {
		diffviewModel.Viewport().ScrollDown(3)
	} else {
		diffviewModel.Viewport().ScrollUp(3)
	}
	return true
}

func (m Model) diffRect(mainH int) (x, y, w, h int, ok bool) {
	if m.diffarea.Fullscreen && m.focus == focusDiff {
		return 0, 0, m.width, mainH, true
	}
	if m.useStackedLayout() {
		filetreeH, diffH := m.splitHeight(mainH)
		return 0, filetreeH, m.width, diffH, true
	}
	filetreeW, diffW := m.splitWidth()
	return filetreeW, 0, diffW, mainH, true
}

func (m *Model) mouseTargetSection(relY, diffH int) *diffview.Model {
	if diffH <= 0 {
		return nil
	}
	expandedH, collapsedH := diffPaneHeights(diffH)
	if m.diffarea.ActiveSection == diffarea.SectionStaged {
		if relY < collapsedH {
			return m.diffarea.SectionModel(diffarea.SectionUnstaged)
		}
		if relY < collapsedH+expandedH {
			return m.diffarea.SectionModel(diffarea.SectionStaged)
		}
		return nil
	}
	if relY < expandedH {
		return m.diffarea.SectionModel(diffarea.SectionUnstaged)
	}
	if relY < expandedH+collapsedH {
		return m.diffarea.SectionModel(diffarea.SectionStaged)
	}
	return nil
}
