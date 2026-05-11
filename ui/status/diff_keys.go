package status

import (
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/status/diffarea"

	tea "charm.land/bubbletea/v2"
)

func (m Model) handleDiffKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.Code == tea.KeyTab {
		m.switchDiffSection()
		return m, nil
	}
	switch msg.String() {
	case "[":
		return m, m.adjustDiffContextLines(-1)
	case "]":
		return m, m.adjustDiffContextLines(1)
	case "esc", "q":
		if m.diff.ActiveSectionModel().DataRef().VisualActive {
			m.diff.DisableVisual()
			return m, nil
		}
		m.focus = focusFiletree
		return m, nil
	case "h", "left":
		m.focus = focusFiletree
		return m, nil
	case "a":
		m.diff.DisableVisual()
		m.diff.ToggleNavMode()
		m.diff.ActiveSectionModel().EnsureActiveVisible(m.diff.NavMode())
	case "v":
		activeModel := m.diff.ActiveSectionModel()
		if m.diff.NavMode() == diffview.NavModeHunk {
			m.diff.SetNavMode(diffview.NavModeLine)
		}
		if !m.diff.ToggleVisual() {
			return m, nil
		}
		activeModel.EnsureActiveVisible(m.diff.NavMode())
	case "f":
		m.diff.Fullscreen = !m.diff.Fullscreen
		var cmd tea.Cmd
		if m.diff.RenderMode() == diffview.RenderModeSideBySide {
			cmd = m.reloadDiffsForSelection()
		}
		m.syncDiffViewports()
		m.diff.ActiveSectionModel().EnsureActiveVisible(m.diff.NavMode())
		return m, cmd
	case "s":
		return m, m.toggleRenderMode()
	case "w":
		m.diff.SetWrap(!m.diff.Wrap())

		m.syncDiffViewports()
		m.diff.ActiveSectionModel().EnsureActiveVisible(m.diff.NavMode())
	case "r":
		return m, m.refresh()
	case "p":
		m.startPullAction()
		return m, actionPollCmd()
	case "P":
		if err := m.preparePushConfirm(); err != nil {
			m.showGitError(err)
			return m, nil
		}
		return m, nil
	case "b":
		if err := m.prepareRebaseConfirm(); err != nil {
			m.showGitError(err)
			return m, nil
		}
		return m, nil
	case "A":
		if err := m.openAmendConfirm(); err != nil {
			m.showGitError(err)
		}
	case "j", "down":
		m.moveActive(1)
	case "k", "up":
		m.moveActive(-1)
	case "J":
		activeModel := m.diff.ActiveSectionModel()
		activeModel.Viewport().ScrollDown(3)
	case "K":
		activeModel := m.diff.ActiveSectionModel()
		activeModel.Viewport().ScrollUp(3)
	case "ctrl+d":
		m.scrollDiffPage(1)
	case "ctrl+u":
		m.scrollDiffPage(-1)
	case "space", " ":
		cmd := m.applySelection()
		return m, cmd
	case "d":
		if m.diff.ActiveSection == diffarea.SectionStaged {
			cmd := m.applySelection()
			return m, cmd
		}
		m.openDiscardDiffConfirm()
		return m, nil
	case ".":
		if ok, cmd := m.moveToAdjacentFile(1); ok {
			return m, cmd
		}
	case ",":
		if ok, cmd := m.moveToAdjacentFile(-1); ok {
			return m, cmd
		}
	case "e":
		return m, m.cmdEditSelectedFile()
	}
	return m, nil
}
