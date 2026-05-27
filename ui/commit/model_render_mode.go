package commit

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/notify"
)

func (m *Model) toggleRenderMode() tea.Cmd {
	var notifyMsg string
	if m.diffModel.RenderMode() == diffview.RenderModeUnified {
		if !git.DeltaAvailable() {
			return notify.Warning("side-by-side requires delta; staying in unified mode")
		}
		m.diffModel.SetRenderMode(diffview.RenderModeSideBySide)
		notifyMsg = "side-by-side mode"
	} else {
		m.diffModel.SetRenderMode(diffview.RenderModeUnified)
		notifyMsg = "unified mode"
	}
	m.refreshDiff()
	m.syncDiffViewport()
	return notify.Info(notifyMsg)
}
