package log

import (
	"github.com/elentok/gx/git"

	tea "charm.land/bubbletea/v2"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	// ctrl+c quits unconditionally even when a modal is open.
	if kp, ok := msg.(tea.KeyPressMsg); ok && kp.String() == "ctrl+c" {
		return m, tea.Quit
	}
	// Delegate all messages to amend.Model while it's open.
	if m.amendConfirm.IsOpen {
		return m.handleAmendUpdate(msg)
	}

	switch msg := msg.(type) {
	case reloadMsg:
		return m.handleReload(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.help, cmd = m.help.Update(msg)
		return m, cmd
	case tea.FocusMsg:
		return m, m.cmdReload()
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.help.IsOpen {
			m.help, cmd = m.help.Update(msg)
			return m, cmd
		}
		if nextSearch, cmd, result := m.search.Update(msg); result.Handled {
			m.search = nextSearch
			if result.QueryChanged {
				m.recomputeSearchMatches()
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
	}
	return m, nil
}

func (m Model) handleReload(msg reloadMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		return m, nil
	}
	m.err = nil
	m.rows = msg.rows
	m.branchDiverged = msg.branchDiverged
	if msg.focusSubject != "" {
		m.list.SetSelected(0, len(m.rows))
		for i, r := range m.rows {
			if r.commit.Subject == msg.focusSubject {
				m.list.SetSelected(i, len(m.rows))
				break
			}
		}
	} else {
		m.list.SetSelected(m.list.Selected(), len(m.rows))
	}
	m.list.EnsureSelectionVisible(len(m.rows), maxInt(1, m.height-3))
	m.recomputeSearchMatches()
	m.jumpToCurrentMatch()
	return m, nil
}

func (m *Model) jumpToTaggedCommit(step int) {
	if len(m.rows) == 0 || step == 0 {
		return
	}
	for i := m.list.Selected() + step; i >= 0 && i < len(m.rows); i += step {
		if rowHasTag(m.rows[i]) {
			m.list.SetSelected(i, len(m.rows))
			m.list.EnsureSelectionVisible(len(m.rows), maxInt(1, m.height-3))
			return
		}
	}
}

func rowHasTag(r row) bool {
	for _, decoration := range r.commit.Decorations {
		if decoration.Kind == git.RefDecorationTag {
			return true
		}
	}
	return false
}
