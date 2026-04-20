package worktrees

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// enterSearchMode transitions the model into search mode with an empty query.
func (m Model) enterSearchMode() Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Focus()
	m.mode = modeSearch
	m.textInput = ti
	m.searchQuery = ""
	m.searchMatches = nil
	m.searchCursor = 0
	m.statusMsg = ""
	return m
}

// exitSearchMode clears search state and returns to normal mode.
func (m Model) exitSearchMode() Model {
	m.mode = modeNormal
	m.searchQuery = ""
	m.searchMatches = nil
	m.searchCursor = 0
	m.table.SetRows(m.buildRows())
	return m
}

// recomputeSearchMatches rebuilds the searchMatches slice from the current query.
func (m Model) recomputeSearchMatches() Model {
	q := strings.ToLower(m.searchQuery)
	m.searchMatches = nil
	if q == "" {
		return m
	}
	for i, wt := range m.worktrees {
		if strings.Contains(strings.ToLower(wt.Name), q) || strings.Contains(strings.ToLower(wt.Branch), q) {
			m.searchMatches = append(m.searchMatches, i)
		}
	}
	return m
}

// handleSearchKey handles key events while in search mode.
func (m Model) handleSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.SearchClose):
		m = m.exitSearchMode()
		return m, nil

	case key.Matches(msg, keys.SearchNext):
		if len(m.searchMatches) > 0 {
			m.searchCursor = (m.searchCursor + 1) % len(m.searchMatches)
			return m.jumpToSearchCursor()
		}
		return m, nil

	case key.Matches(msg, keys.SearchPrev):
		if len(m.searchMatches) > 0 {
			m.searchCursor = (m.searchCursor - 1 + len(m.searchMatches)) % len(m.searchMatches)
			return m.jumpToSearchCursor()
		}
		return m, nil
	}

	var tiCmd tea.Cmd
	m.textInput, tiCmd = m.textInput.Update(msg)
	m.searchQuery = m.textInput.Value()
	m = m.recomputeSearchMatches()
	if len(m.searchMatches) > 0 {
		m.searchCursor = 0
		return m.jumpToSearchCursor()
	}
	m.table.SetRows(m.buildRows())
	return m, tiCmd
}

// jumpToSearchCursor moves the table cursor to the current search match and
// returns the sidebar-reload command.
func (m Model) jumpToSearchCursor() (Model, tea.Cmd) {
	idx := m.searchMatches[m.searchCursor]
	m.table.SetCursor(idx)
	m.table.SetRows(m.buildRows())
	return m, cmdLoadSidebarData(m.repo, m.worktrees[idx])
}

