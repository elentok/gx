package status

import (
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/list"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/status/diffarea"

	tea "charm.land/bubbletea/v2"
)

func (m Model) delegateToDiff(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Search routing first.
	cmd, handled := m.diffarea.UpdateActive(msg)
	if handled {
		if m.currentDiffSearch().Mode() == search.SearchModeResults {
			m.diffarea.SetNavMode(diffview.NavModeLine)
		}
		return m, cmd
	}

	match, consumed := m.diffarea.Keys().Process(msg)
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
		m.diffarea.ActiveSectionModel().ScrollViewport(3)
	case diffarea.BindingScrollUp:
		m.diffarea.ActiveSectionModel().ScrollViewport(-3)
	case diffarea.BindingPageDown:
		m.diffarea.ScrollPage(list.DefaultScroll)
	case diffarea.BindingPageUp:
		m.diffarea.ScrollPage(-list.DefaultScroll)
	case diffarea.BindingNavMode:
		m.diffarea.DisableVisual()
		m.diffarea.ToggleNavMode()
		m.diffarea.ActiveSectionModel().EnsureActiveVisible(m.diffarea.NavMode())
	case diffarea.BindingVisual:
		if m.diffarea.NavMode() == diffview.NavModeHunk {
			m.diffarea.SetNavMode(diffview.NavModeLine)
		}
		if !m.diffarea.ToggleVisual() {
			return m, nil
		}
		m.diffarea.ActiveSectionModel().EnsureActiveVisible(m.diffarea.NavMode())
	case diffarea.BindingFullscreen:
		m.diffarea.Fullscreen = !m.diffarea.Fullscreen
		var reloadCmd tea.Cmd
		if m.diffarea.RenderMode() == diffview.RenderModeSideBySide {
			reloadCmd = m.reloadDiffsForSelection()
		}
		m.syncDiffViewports()
		m.diffarea.ActiveSectionModel().EnsureActiveVisible(m.diffarea.NavMode())
		return m, reloadCmd
	case diffarea.BindingWrap:
		m.diffarea.SetWrap(!m.diffarea.Wrap())
		m.syncDiffViewports()
		m.diffarea.ActiveSectionModel().EnsureActiveVisible(m.diffarea.NavMode())
	case diffarea.BindingBack:
		if m.diffarea.ActiveSectionModel().DataRef().VisualActive {
			m.diffarea.DisableVisual()
		} else {
			m.focus = focusFiletree
		}
	case diffarea.BindingApply:
		return m, m.applySelection()
	case diffarea.BindingDiscard:
		if m.diffarea.ActiveSection == diffarea.SectionStaged {
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
