package worktrees

import (
	tea "charm.land/bubbletea/v2"
)

func (m Model) enterHelpMode() Model {
	m.helpModel.Open(m.width, m.height)
	m.mode = modeHelp
	return m
}

func (m Model) handleHelpKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.helpModel, cmd = m.helpModel.Update(msg)
	if !m.helpModel.IsOpen {
		m.mode = modeNormal
	}
	return m, cmd
}

func (m Model) helpModalView() string {
	return m.helpModel.View()
}
