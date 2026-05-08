package search

import (
	tea "charm.land/bubbletea/v2"
)

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch m.mode {
		case SearchModeNone:
			return m.handleKeyPressOutOfSearch(msg)
		case SearchModeInput:
			return m.handleKeyPressInput(msg)
		case SearchModeResults:
			return m.handleKeyPressResults(msg)
		}
	}

	return m, nil, false
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
