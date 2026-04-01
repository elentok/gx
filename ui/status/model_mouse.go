package stage

import tea "charm.land/bubbletea/v2"

func (m *Model) handleMouseWheel(msg tea.MouseWheelMsg) bool {
	if m.runningOpen || m.confirmOpen || m.errorOpen || m.helpOpen || m.searchMode != searchModeNone {
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

func (m *Model) scrollDiffByMouse(x, y, dir int) bool {
	mainH := m.height - 1
	if mainH < 1 || y < 0 || y >= mainH || x < 0 || x >= m.width {
		return false
	}

	diffX, diffY, diffW, diffH, ok := m.diffRect(mainH)
	if !ok || x < diffX || x >= diffX+diffW || y < diffY || y >= diffY+diffH {
		return false
	}

	sec := m.mouseTargetSection(y-diffY, diffH)
	if sec == nil {
		return false
	}
	if dir > 0 {
		sec.viewport.ScrollDown(3)
	} else {
		sec.viewport.ScrollUp(3)
	}
	return true
}

func (m Model) diffRect(mainH int) (x, y, w, h int, ok bool) {
	if m.diffFullscreen && m.focus == focusDiff {
		return 0, 0, m.width, mainH, true
	}
	if m.useStackedLayout() {
		statusH, diffH := m.splitHeight(mainH)
		return 0, statusH, m.width, diffH, true
	}
	statusW, diffW := m.splitWidth()
	return statusW, 0, diffW, mainH, true
}

func (m *Model) mouseTargetSection(relY, diffH int) *sectionState {
	hasUnstaged := len(m.unstaged.viewLines) > 0 || sectionHasBinaryDiff(m.unstaged)
	hasStaged := len(m.staged.viewLines) > 0 || sectionHasBinaryDiff(m.staged)
	if !hasUnstaged && !hasStaged {
		return nil
	}
	if m.diffFullscreen {
		return m.currentSection()
	}
	if hasUnstaged && !hasStaged {
		return &m.unstaged
	}
	if hasStaged && !hasUnstaged {
		return &m.staged
	}

	topH := diffH / 2
	if topH < 5 {
		topH = 5
	}
	bottomH := diffH - topH
	if bottomH < 5 {
		bottomH = 5
		topH = diffH - bottomH
	}
	if relY < topH {
		return &m.unstaged
	}
	return &m.staged
}
