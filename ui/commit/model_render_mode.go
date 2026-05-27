package commit

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/diffview"
)

func (m *Model) toggleRenderMode() tea.Cmd {
	if m.diffModel.RenderMode() == diffview.RenderModeUnified {
		if !git.DeltaAvailable() {
			return nil
		}
		m.diffModel.SetRenderMode(diffview.RenderModeSideBySide)
	} else {
		m.diffModel.SetRenderMode(diffview.RenderModeUnified)
	}
	m.refreshDiff()
	m.syncDiffViewport()
	return nil
}
