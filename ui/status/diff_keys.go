package status

import (
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/status/diffarea"

	tea "charm.land/bubbletea/v2"
)

func (m Model) delegateToDiff(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var result diffview.UpdateResult
	m.diffarea, cmd, result = m.diffarea.Update(msg)
	if result.Handled {
		m.diffarea.SetNavMode(m.diffarea.ActiveSectionModel().NavMode())
		if result.SearchConfirmed {
			m.syncSearchToInactivePane()
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
	case diffarea.BindingFullscreen:
		m.diffarea.Fullscreen = !m.diffarea.Fullscreen
		var reloadCmd tea.Cmd
		if m.diffarea.RenderMode() == diffview.RenderModeSideBySide {
			reloadCmd = m.reloadDiffsForSelection()
		}
		m.syncDiffViewports()
		m.diffarea.ActiveSectionModel().EnsureActiveVisible(m.diffarea.NavMode())
		return m, reloadCmd
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
		return m, m.openDiscardDiffConfirm()
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
