package status

import (
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/status/filetree"

	tea "charm.land/bubbletea/v2"
)

const (
	filetreeCat = "Filetree"
)

var (
	toggleStageBinding = keys.Binding{ID: filetree.BindingToggleStage, Seq: []string{"space"}, Categories: []string{filetreeCat}, Title: "toggle stage"}
	discardBinding     = keys.Binding{ID: filetree.BindingDiscard, Seq: []string{"d"}, Categories: []string{filetreeCat}, Title: "discard"}
)

func delegateToChild(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.focus == focusFiletree {
		return delegateToFiletree(m, msg)
	}
	return m.delegateToDiff(msg)
}

func delegateToFiletree(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	var childCmd tea.Cmd
	var result filetree.Result
	m.filetree, childCmd, result = m.filetree.Update(msg)

	switch {
	case result.Back:
		if m.settings.EnableNavigation {
			return m, nav.Back()
		}
		return m, nil
	case result.PageMoved:
		return m, m.diffLoader.scheduleDiffReload()
	case result.ToggleStage:
		return m, toggleStageStatusEntry(&m)
	case result.Discard:
		openDiscardStatusConfirm(&m)
		return m, nil
	}

	var actionCmd tea.Cmd
	if result.Rebuilt {
		actionCmd = m.reloadDiffsForSelection()
	}
	if result.OpenSelected {
		actionCmd = openSelectedFile(&m)
	}
	selectionChanged := result.SelectionChanged
	if selectionChanged {
		m.filetree.SyncListSelection()
		onFiletreeSelectionChanged(&m)
	}
	if childCmd != nil {
		if handledModel, handledCmd, handled := handleFileTreeChildMsg(m, childCmd()); handled {
			if selectionChanged {
				return handledModel, tea.Batch(actionCmd, handledCmd, m.diffLoader.scheduleDiffReload())
			}
			return handledModel, tea.Batch(actionCmd, handledCmd)
		}
		if selectionChanged {
			return m, tea.Batch(actionCmd, msgCmd(childCmd()), m.diffLoader.scheduleDiffReload())
		}
		return m, tea.Batch(actionCmd, msgCmd(childCmd()))
	}
	if selectionChanged {
		return m, tea.Batch(actionCmd, m.diffLoader.scheduleDiffReload())
	}
	return m, actionCmd
}

func handleFileTreeChildMsg(m Model, msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg.(type) {
	case search.JumpToMatchMsg:
		model, cmd := m.handleJumpToMatch(msg.(search.JumpToMatchMsg))
		return model, cmd, true
	case search.SearchQueryUpdatedMsg:
		model, cmd := handleSearchQueryUpdated(m, msg.(search.SearchQueryUpdatedMsg))
		return model, cmd, true
	default:
		return m, nil, false
	}
}

func msgCmd(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}
