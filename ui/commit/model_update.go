package commit

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/reword"
	"github.com/elentok/gx/ui/search"
)

func (m Model) Update(msg tea.Msg) (next tea.Model, cmd tea.Cmd) {
	// ctrl+c quits unconditionally even when a modal is open.
	if kp, ok := msg.(tea.KeyPressMsg); ok && kp.String() == "ctrl+c" {
		return m, tea.Quit
	}
	// Delegate all messages to amend.Model while it's open.
	if m.amendConfirm.IsOpen {
		return m.handleAmendUpdate(msg)
	}
	// Delegate all messages to reword.Model while it's running.
	if m.reword.IsOpen {
		return m.handleRewordRunningUpdate(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case tea.MouseWheelMsg:
		return m.handleMouseWheel(msg)
	case gotoPRMsg:
		return m.handleGotoPR(msg)
	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)
	case editFileFinishedMsg:
		return m.handleEditFileFinished(msg)
	case editCommentFinishedMsg:
		return m.handleEditCommentFinished(msg)
	case reword.EditorFinishedMsg:
		return m.handleRewordEditorDone(msg.Err)
	}
	return m, nil
}

func (m Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.ready = true
	m.syncDiffViewport()
	return m, nil
}

func (m Model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	if m.help.IsOpen {
		var cmd tea.Cmd
		m.help, cmd = m.help.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	var result search.Result
	if m.focusDiff {
		m.search, cmd, result = m.search.Update(msg)
	} else {
		fileSearch := m.fileTreeModel.Search()
		var updated search.Model
		updated, cmd, result = fileSearch.Update(msg)
		*fileSearch = updated
	}
	if result.Handled {
		if result.QueryChanged {
			if m.focusDiff {
				m.search.SetMatches(m.computeDiffSearchMatches(m.search.Query()))
			} else {
				m.fileTreeModel.RecomputeSearchMatches(m.fileEntrySearchText)
			}
		}
		if result.QueryChanged || result.CursorChanged {
			if m.focusDiff {
				m.jumpToCurrentDiffMatch()
			} else if m.fileTreeModel.FocusCurrentSearchMatch() {
				m.refreshDiff()
			}
		}
		return m, cmd
	}
	if m.focusDiff && (len(m.keys.Prefix()) == 0 || m.diffModel.HasPendingChord()) {
		updated, diffCmd, diffResult := m.diffModel.Update(msg)
		m.diffModel = updated
		if diffResult.Handled && !diffResult.ChordInProgress {
			m.keys.Reset()
			m.syncSearchCursorFromDiffFocus()
			if diffResult.NeedsReload {
				m.refreshDiff()
				m.syncDiffViewport()
			}
			return m, diffCmd
		}
	}
	match, consumed := m.keys.Process(msg)
	if match != nil {
		return m.dispatchBinding(match.ID)
	}
	if consumed {
		return m, nil
	}
	return m, nil
}
