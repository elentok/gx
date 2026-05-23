package commit

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/reword"
)

func (m Model) Update(msg tea.Msg) (next tea.Model, cmd tea.Cmd) {
	prevRoute, prevOK := m.CurrentViewState()
	// Run once on every return path so route-change emission stays centralized.
	// We compare pre/post route state and append ViewStateChanged only when needed.
	defer func() {
		nextModel, ok := next.(Model)
		if !ok {
			return
		}
		route, routeOK := nextModel.CurrentViewState()
		cmd = nav.AppendViewStateChanged(cmd, m.settings.EnableNavigation, prevRoute, prevOK, route, routeOK)
	}()

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
	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)
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
	newSearch, cmd, result := m.search.Update(msg)
	m.search = newSearch
	if result.Handled {
		if result.Activated {
			m.searchScope = searchScopeSidebar
			if m.focusDiff {
				m.searchScope = searchScopeDiff
			}
		}
		if result.QueryChanged {
			m.search.SetMatches(m.computeSearchMatches(m.search.Query()))
		}
		if result.QueryChanged || result.CursorChanged {
			m.jumpToCurrentMatch()
		}
		return m, cmd
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
