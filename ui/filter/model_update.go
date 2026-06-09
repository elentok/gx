package filter

import (
	tea "charm.land/bubbletea/v2"
)

// Result describes what changed during a single Update call, mirroring
// ui/search's host-facing shape minus the match-engine bits.
type Result struct {
	Handled      bool // key was consumed by the filter
	Activated    bool // transitioned None → Input (filter session just started)
	QueryChanged bool // query text changed this update
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, Result) {
	prevMode := m.mode
	prevQuery := m.query

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		newModel, cmd, handled := m.handleKeyPress(msg)
		return newModel, cmd, Result{
			Handled:      handled,
			Activated:    handled && prevMode == ModeNone && newModel.mode != ModeNone,
			QueryChanged: handled && newModel.query != prevQuery,
		}
	}

	return m, nil, Result{}
}

func (m Model) handleKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.mode != ModeInput {
		if msg.String() == "/" {
			m.Start()
			return m, nil, true
		}
		return m, nil, false
	}

	switch msg.String() {
	case "esc":
		m.Clear()
		return m, nil, true
	case "enter":
		m.keepAndDefocus()
		return m, nil, true
	}

	var cmd tea.Cmd
	m.textinput, cmd = m.textinput.Update(msg)
	m.query = m.textinput.Value()
	return m, tea.Batch(cmd, createFilterChangedCmd(m.query)), true
}
