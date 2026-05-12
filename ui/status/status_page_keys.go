package status

import (
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/filetree"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/search"

	tea "charm.land/bubbletea/v2"
)

func (m Model) handleFiletreeKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		if m.settings.EnableNavigation {
			return m, nav.Back()
		}
		return m, tea.Quit
	case "esc":
		if m.settings.EnableNavigation {
			return m, nav.Back()
		}
		return m, nil
	case "l", "right":
		return m, m.enterDiffFromStatus(false)
	case "enter":
		return m, m.enterDiffFromStatus(false)
	case "tab":
		m.switchDiffSection()
		return m, nil
	case "h", "left":
		return m, nil
	case "[":
		return m, m.adjustDiffContextLines(-1)
	case "]":
		return m, m.adjustDiffContextLines(1)
	case "r":
		return m, m.refresh()
	case "s":
		return m, m.toggleRenderMode()
	case "p":
		return m.startPullAction()
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
	case "ctrl+d":
		if m.scrollFiletreePage(1) {
			return m, m.scheduleDiffReload()
		}
	case "ctrl+u":
		if m.scrollFiletreePage(-1) {
			return m, m.scheduleDiffReload()
		}
	case "space", " ":
		return m, m.toggleStageStatusEntry()
	case "d":
		m.openDiscardStatusConfirm()
	case "e":
		return m, m.cmdEditSelectedFile()
	}
	return m, nil
}

func (m Model) handleFocusedChildKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch m.focus {
	case focusFiletree:
		if msg.Code == tea.KeyTab {
			m.switchDiffSection()
			return m, nil, true
		}
		switch msg.String() {
		case "h", "left", "l", "right":
			return m, nil, false
		case "enter":
			if m.fileTreeModel.Search().IsActive() {
				break
			}
			return m, nil, false
		}
		m.reconcileFileTreeFromStatusState()
		updatedFileTree, childCmd, handled := m.fileTreeModel.Update(msg)
		selectionChanged := updatedFileTree.SelectedIndex() != m.fileTreeModel.SelectedIndex()
		m.fileTreeModel = updatedFileTree
		if !handled {
			return m, nil, false
		}
		if selectionChanged {
			m.page.selected = m.fileTreeModel.SelectedIndex()
			m.onFiletreeSelectionChanged()
		}
		if childCmd != nil {
			if handledModel, handledCmd, handled := m.handleFileTreeChildMsg(childCmd()); handled {
				if selectionChanged {
					return handledModel, tea.Batch(handledCmd, m.scheduleDiffReload()), true
				}
				return handledModel, handledCmd, true
			}
			if selectionChanged {
				return m, tea.Batch(msgCmd(childCmd()), m.scheduleDiffReload()), true
			}
			return m, msgCmd(childCmd()), true
		}
		if selectionChanged {
			return m, m.scheduleDiffReload(), true
		}
		return m, nil, true
	case focusDiff:
		cmd, handled := m.diff.UpdateActive(msg)
		if !handled {
			return m, nil, false
		}
		if m.currentDiffSearch().Mode() == search.SearchModeResults && m.focus == focusDiff {
			m.diff.SetNavMode(diffview.NavModeLine)

		}
		return m, cmd, true
	default:
		return m, nil, false
	}
}

func (m Model) handleFileTreeChildMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg.(type) {
	case filetree.RebuildRequestedMsg:
		model, cmd := m.handleFileTreeRebuildRequested()
		return model, cmd, true
	case filetree.OpenSelectedMsg:
		model, cmd := m.handleFileTreeOpenSelected()
		return model, cmd, true
	case search.JumpToMatchMsg:
		model, cmd := m.handleJumpToMatch(msg.(search.JumpToMatchMsg))
		return model, cmd, true
	case search.SearchQueryUpdatedMsg:
		model, cmd := m.handleSearchQueryUpdated(msg.(search.SearchQueryUpdatedMsg))
		return model, cmd, true
	default:
		return m, nil, false
	}
}

func msgCmd(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}
