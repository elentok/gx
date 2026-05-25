package search

import (
	tea "charm.land/bubbletea/v2"
)

// Result describes what changed during a single Update call.
type Result struct {
	Handled       bool // key was consumed by search
	Activated     bool // transitioned None → Input (search session just started)
	QueryChanged  bool // query text changed this update
	CursorChanged bool // match cursor moved this update (n / N)
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, Result) {
	prevMode := m.mode
	prevQuery := m.query
	prevCursor := m.cursor

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		var newModel Model
		var cmd tea.Cmd
		var handled bool
		switch m.mode {
		case SearchModeNone:
			newModel, cmd, handled = m.handleKeyPressOutOfSearch(msg)
		case SearchModeInput:
			newModel, cmd, handled = m.handleKeyPressInput(msg)
		case SearchModeResults:
			newModel, cmd, handled = m.handleKeyPressResults(msg)
		}
		return newModel, cmd, Result{
			Handled:       handled,
			Activated:     handled && prevMode == SearchModeNone && newModel.mode != SearchModeNone,
			QueryChanged:  handled && newModel.query != prevQuery,
			CursorChanged: handled && newModel.cursor != prevCursor,
		}
	}

	return m, nil, Result{}
}

func (m Model) handleKeyPressOutOfSearch(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	switch msg.String() {
	case "/":
		m.Start("")
		return m, nil, true
	}

	return m, nil, false
}

func (m Model) handleKeyPressInput(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg.String() {
	case "/":
	case "esc":
		m.DismissAndClear()
		return m, nil, true

	case "enter":
		m.DismissAndKeepResults()
		return m, nil, true
	}

	m.textinput, cmd = m.textinput.Update(msg)
	cmds = append(cmds, cmd)

	m.query = m.textinput.Value()

	// recompute matches
	cmds = append(cmds, createSearchQueryUpdatedCmd(m.query))

	return m, tea.Batch(cmds...), true
}

func (m Model) handleKeyPressResults(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	switch msg.String() {
	case "esc":
		m.DismissAndClear()
		return m, nil, true
	case "n":
		if m.cursor+1 < len(m.matches) {
			m.cursor = m.cursor + 1
			return m, jumpToMatchCmd(m.matches[m.cursor]), true
		}
		return m, nil, true
	case "N", "shift+n":
		if m.cursor > 0 {
			m.cursor = m.cursor - 1
			return m, jumpToMatchCmd(m.matches[m.cursor]), true
		}
		return m, nil, true
	}

	return m, nil, false
}
