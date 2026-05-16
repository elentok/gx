package status

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/notify"
)

func (m Model) deltaRenderWidth() int {
	mainH := m.height - 1
	if mainH < 4 {
		mainH = 4
	}
	_, diffW := m.splitWidth()
	if m.diffarea.Fullscreen && m.focus == focusDiff {
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
	var notifyMsg string
	if m.diffarea.RenderMode() == diffview.RenderModeUnified {
		if !git.DeltaAvailable() {
			return notify.Warning("side-by-side requires delta; staying in unified mode")
		}
		m.diffarea.SetRenderMode(diffview.RenderModeSideBySide)
		notifyMsg = "side-by-side mode"
	} else {
		m.diffarea.SetRenderMode(diffview.RenderModeUnified)
		notifyMsg = "unified mode"
	}
	cmd := m.reloadDiffsForSelection()
	m.syncDiffViewports()
	m.diffarea.ActiveSectionModel().EnsureActiveVisible(m.diffarea.NavMode())
	return tea.Batch(notify.Info(notifyMsg), cmd)
}

func (m Model) renderModeLabel() string {
	if m.diffarea.RenderMode() == diffview.RenderModeSideBySide {
		return "side-by-side"
	}
	return "unified"
}
