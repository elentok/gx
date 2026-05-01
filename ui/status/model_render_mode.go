package status

import "github.com/elentok/gx/git"
import "github.com/charmbracelet/x/ansi"

func (m Model) deltaRenderWidth() int {
	mainH := m.height - 1
	if mainH < 4 {
		mainH = 4
	}
	_, diffW := m.splitWidth()
	if m.diffFullscreen && m.focus == focusDiff {
		diffW = m.width
	}
	innerW := maxInt(1, diffW-2)

	markW := ansi.StringWidth("▌ ")
	indicator := "* "
	if m.settings.UseNerdFontIcons {
		indicator = "󰍉 "
	}
	indicatorW := ansi.StringWidth(indicator)

	return maxInt(1, innerW-markW-indicatorW)
}

func (m *Model) toggleRenderMode() {
	if m.renderMode == renderUnified {
		if !git.DeltaAvailable() {
			m.setStatus("side-by-side requires delta; staying in unified mode")
			return
		}
		m.renderMode = renderSideBySide
		m.setStatus("side-by-side mode")
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

func (m Model) renderModeLabel() string {
	if m.renderMode == renderSideBySide {
		return "side-by-side"
	}
	return "unified"
}
