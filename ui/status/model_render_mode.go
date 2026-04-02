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
		m.setStatus("side-by-side mode (read-only)")
	} else {
		m.renderMode = renderUnified
		m.setStatus("unified mode")
	}
	m.reloadDiffsForSelection()
	m.syncDiffViewports()
	m.ensureActiveVisible(m.currentSection())
}

func (m Model) isSideBySideReadOnly() bool {
	return m.renderMode == renderSideBySide
}

func (m *Model) blockIfSideBySideReadOnly() bool {
	if !m.isSideBySideReadOnly() {
		return false
	}
	m.setStatus("side-by-side is read-only; press s for interactive mode")
	return true
}

func (m Model) renderModeLabel() string {
	if m.renderMode == renderSideBySide {
		return "side-by-side"
	}
	return "unified"
}
