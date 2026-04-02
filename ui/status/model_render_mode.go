package stage

func (m Model) deltaRenderWidth() int {
	mainH := m.height - 1
	if mainH < 4 {
		mainH = 4
	}
	_, diffW := m.splitWidth()
	if m.diffFullscreen && m.focus == focusDiff {
		diffW = m.width
	}
	vpW := maxInt(1, diffW-4)
	return vpW
}

func (m *Model) toggleRenderMode() {
	if m.renderMode == renderUnified {
		m.renderMode = renderSideBySide
		m.navMode = navHunk
		sec := m.currentSection()
		sec.visualActive = false
		sec.visualAnchor = sec.activeLine
		m.setStatus("side-by-side mode (hunk-only)")
	} else {
		m.renderMode = renderUnified
		m.setStatus("unified mode")
	}
	m.reloadDiffsForSelection()
	m.syncDiffViewports()
	m.ensureActiveVisible(m.currentSection())
}

func (m Model) isSideBySideMode() bool {
	return m.renderMode == renderSideBySide
}

func (m *Model) blockIfSideBySideLineAction() bool {
	if !m.isSideBySideMode() {
		return false
	}
	m.setStatus("side-by-side supports hunk mode only; press s for full interactive mode")
	return true
}

func (m Model) renderModeLabel() string {
	if m.renderMode == renderSideBySide {
		return "side-by-side"
	}
	return "unified"
}
