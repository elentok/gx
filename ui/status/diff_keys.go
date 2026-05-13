package status

import (
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/status/diffarea"

	tea "charm.land/bubbletea/v2"
)

func (m Model) delegateToDiff(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Search routing first.
	cmd, handled := m.diff.UpdateActive(msg)
	if handled {
		if m.currentDiffSearch().Mode() == search.SearchModeResults {
			m.diff.SetNavMode(diffview.NavModeLine)
		}
		return m, cmd
	}

	match, consumed := m.diff.Keys().Process(msg)
	if consumed && match == nil {
		return m, nil // chord in progress
	}
	if match == nil {
		return m, nil
	}

	switch match.ID {
	case diffarea.BindingMoveDown:
		m.moveActive(1)
	case diffarea.BindingMoveUp:
		m.moveActive(-1)
	case diffarea.BindingScrollDown:
		m.diff.ActiveSectionModel().Viewport().ScrollDown(3)
	case diffarea.BindingScrollUp:
		m.diff.ActiveSectionModel().Viewport().ScrollUp(3)
	case diffarea.BindingPageDown:
		m.diff.ScrollPage(1)
	case diffarea.BindingPageUp:
		m.diff.ScrollPage(-1)
	case diffarea.BindingNavMode:
		m.diff.DisableVisual()
		m.diff.ToggleNavMode()
		m.diff.ActiveSectionModel().EnsureActiveVisible(m.diff.NavMode())
	case diffarea.BindingVisual:
		if m.diff.NavMode() == diffview.NavModeHunk {
			m.diff.SetNavMode(diffview.NavModeLine)
		}
		if !m.diff.ToggleVisual() {
			return m, nil
		}
		m.diff.ActiveSectionModel().EnsureActiveVisible(m.diff.NavMode())
	case diffarea.BindingFullscreen:
		m.diff.Fullscreen = !m.diff.Fullscreen
		var reloadCmd tea.Cmd
		if m.diff.RenderMode() == diffview.RenderModeSideBySide {
			reloadCmd = m.reloadDiffsForSelection()
		}
		m.syncDiffViewports()
		m.diff.ActiveSectionModel().EnsureActiveVisible(m.diff.NavMode())
		return m, reloadCmd
	case diffarea.BindingWrap:
		m.diff.SetWrap(!m.diff.Wrap())
		m.syncDiffViewports()
		m.diff.ActiveSectionModel().EnsureActiveVisible(m.diff.NavMode())
	case diffarea.BindingBack:
		if m.diff.ActiveSectionModel().DataRef().VisualActive {
			m.diff.DisableVisual()
		} else {
			m.focus = focusFiletree
		}
	case diffarea.BindingApply:
		return m, m.applySelection()
	case diffarea.BindingDiscard:
		if m.diff.ActiveSection == diffarea.SectionStaged {
			return m, m.applySelection()
		}
		m.openDiscardDiffConfirm()
	case diffarea.BindingNextFile:
		if ok, cmd := m.moveToAdjacentFile(1); ok {
			return m, cmd
		}
	case diffarea.BindingPrevFile:
		if ok, cmd := m.moveToAdjacentFile(-1); ok {
			return m, cmd
		}
	}
	return m, nil
}
