package status

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/diffview"

	tea "charm.land/bubbletea/v2"
)

func (m Model) deltaRenderWidth() int {
	mainH := m.height - 1
	if mainH < 4 {
		mainH = 4
	}
	_, diffW := m.splitWidth()
	if m.diff.DiffFullscreen && m.focus == focusDiff {
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

func (m *Model) toggleRenderMode() tea.Cmd {
	if m.diff.RenderMode() == diffview.RenderModeUnified {
		if !git.DeltaAvailable() {
			m.setStatus("side-by-side requires delta; staying in unified mode")
			return nil
		}
		m.diff.SetRenderMode(diffview.RenderModeSideBySide)
		m.setStatus("side-by-side mode")
	} else {
		m.diff.SetRenderMode(diffview.RenderModeUnified)
		m.setStatus("unified mode")
	}

	cmd := m.reloadDiffsForSelection()
	m.syncDiffViewports()
	m.diff.ActiveSectionModel().EnsureActiveVisible(m.diff.NavMode())
	return cmd
}

func (m Model) isSideBySideMode() bool {
	return m.diff.RenderMode() == diffview.RenderModeSideBySide
}

func (m Model) renderModeLabel() string {
	if m.diff.RenderMode() == diffview.RenderModeSideBySide {
		return "side-by-side"
	}
	return "unified"
}
