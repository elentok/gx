package status

import (
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/search"

	tea "charm.land/bubbletea/v2"
)

const (
	bindingToggleStage keys.BindingID = "toggle-stage"
	bindingDiscard     keys.BindingID = "discard"

	filetreeCat = "Filetree"
)

var (
	toggleStageBinding = keys.Binding{ID: bindingToggleStage, Seq: []string{"space"}, Categories: []string{filetreeCat}, Title: "toggle stage"}
	discardBinding     = keys.Binding{ID: bindingDiscard, Seq: []string{"d"}, Categories: []string{filetreeCat}, Title: "discard"}
)

func (m Model) delegateToChild(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.focus == focusFiletree {
		return m.delegateToFiletree(msg)
	}
	return m.delegateToDiff(msg)
}

func (m Model) delegateToFiletree(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	var childCmd tea.Cmd
	var result FiletreeResult
	m.filetree, childCmd, result = m.filetree.Update(msg)

	switch {
	case result.Back:
		if m.settings.EnableNavigation {
			return m, nav.Back()
		}
		return m, nil
	case result.PageMoved:
		return m, m.scheduleDiffReload()
	case result.ToggleStage:
		return m, m.toggleStageStatusEntry()
	case result.Discard:
		m.openDiscardStatusConfirm()
		return m, nil
	}

	var actionCmd tea.Cmd
	if result.Rebuilt {
		actionCmd = m.reloadDiffsForSelection()
	}
	if result.OpenSelected {
		actionCmd = m.openSelectedFile()
	}
	selectionChanged := result.SelectionChanged
	if selectionChanged {
		m.filetree.syncListSelection()
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

func (m Model) handleFileTreeChildMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
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
