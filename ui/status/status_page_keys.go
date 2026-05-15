package status

import (
	"github.com/elentok/gx/ui/filetree"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/search"

	tea "charm.land/bubbletea/v2"
)

func (m Model) delegateToChild(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.focus == focusFiletree {
		return m.delegateToFiletree(msg)
	}
	return m.delegateToDiff(msg)
}

func (m Model) delegateToFiletree(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.reconcileFileTreeFromStatusState()
	updatedFileTree, childCmd, result := m.fileTreeModel.Update(msg)
	selectionChanged := updatedFileTree.SelectedIndex() != m.fileTreeModel.SelectedIndex()
	m.fileTreeModel = updatedFileTree

	if !result.Handled {
		match, consumed := m.fileTreeModel.Keys().Process(msg)
		if consumed && match == nil {
			return m, nil // chord in progress
		}
		if match != nil {
			switch match.ID {
			case filetree.BindingBack:
				if m.settings.EnableNavigation {
					return m, nav.Back()
				}
			case filetree.BindingPageDown:
				if m.scrollFiletreePage(1) {
					return m, m.scheduleDiffReload()
				}
			case filetree.BindingPageUp:
				if m.scrollFiletreePage(-1) {
					return m, m.scheduleDiffReload()
				}
			case filetree.BindingToggleStage:
				return m, m.toggleStageStatusEntry()
			case filetree.BindingDiscard:
				m.openDiscardStatusConfirm()
			}
		}
		return m, nil
	}

	var actionCmd tea.Cmd
	if result.RebuildRequested {
		m, actionCmd = m.handleFileTreeRebuildRequested()
	}
	if result.OpenSelected {
		m, actionCmd = m.handleFileTreeOpenSelected()
	}
	if selectionChanged {
		m.statusData.listState.SetSelected(m.fileTreeModel.SelectedIndex(), len(m.statusData.statusEntries))
		m.onFiletreeSelectionChanged()
	}
	if childCmd != nil {
		if handledModel, handledCmd, handled := m.handleFileTreeChildMsg(childCmd()); handled {
			if selectionChanged {
				return handledModel, tea.Batch(actionCmd, handledCmd, m.scheduleDiffReload())
			}
			return handledModel, tea.Batch(actionCmd, handledCmd)
		}
		if selectionChanged {
			return m, tea.Batch(actionCmd, msgCmd(childCmd()), m.scheduleDiffReload())
		}
		return m, tea.Batch(actionCmd, msgCmd(childCmd()))
	}
	if selectionChanged {
		return m, tea.Batch(actionCmd, m.scheduleDiffReload())
	}
	return m, actionCmd
}

func (m Model) handleFileTreeChildMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg.(type) {
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
