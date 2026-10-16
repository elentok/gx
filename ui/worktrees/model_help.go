package worktrees

import (
	tea "charm.land/bubbletea/v2"
)

func (m Model) enterHelpMode() Model {
	m.helpModel.Open(m.width, m.height)
	m.mode = modeHelp
	return m
}

// handleOpenHelp answers the title row's help click. Search is the one mode
// help may replace: every other mode is a dialog the click must not cover.
func (m Model) handleOpenHelp() Model {
	if m.ModalOpen() || (m.mode != modeNormal && m.mode != modeSearch) {
		return m
	}
	if m.mode == modeSearch {
		m = m.exitSearchMode()
	}
	return m.enterHelpMode()
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
