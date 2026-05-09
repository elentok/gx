package status

import (
	"github.com/elentok/gx/ui/filetree"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

func (m Model) handleStatusKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.syncFileTreeModel()

	if msg.Code == tea.KeyTab {
		m.cycleFrameForward()
		return m, nil
	}

	if updatedFileTree, childCmd := m.fileTreeModel.Update(msg); updatedFileTree.SelectedIndex() != m.fileTreeModel.SelectedIndex() {
		m.fileTreeModel = updatedFileTree
		m.selected = m.fileTreeModel.SelectedIndex()
		m.onStatusSelectionChanged()
		if childCmd != nil {
			if handledModel, handledCmd, handled := m.handleFileTreeIntentMsg(childCmd()); handled {
				return handledModel, tea.Batch(handledCmd, m.scheduleDiffReload())
			}
		}
		return m, m.scheduleDiffReload()
	} else {
		m.fileTreeModel = updatedFileTree
		if childCmd != nil {
			if handledModel, handledCmd, handled := m.handleFileTreeIntentMsg(childCmd()); handled {
				return handledModel, handledCmd
			}
		}
	}

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
		if m.scrollStatusPage(1) {
			return m, m.scheduleDiffReload()
		}
	case "ctrl+u":
		if m.scrollStatusPage(-1) {
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

func (m Model) handleFileTreeIntentMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg.(type) {
	case filetree.RebuildRequestedMsg:
		model, cmd := m.handleFileTreeRebuildRequested()
		return model, cmd, true
	case filetree.OpenSelectedMsg:
		model, cmd := m.handleFileTreeOpenSelected()
		return model, cmd, true
	default:
		return m, nil, false
	}
}
