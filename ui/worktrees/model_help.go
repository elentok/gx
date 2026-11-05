package worktrees

import "github.com/elentok/gx/ui/worktrees/modal"

// handleOpenHelp answers the title row's help click. Search is the one mode
// help may replace: every other mode is a dialog the click must not cover.
func (m Model) handleOpenHelp() Model {
	if m.ModalOpen() {
		return m
	}
	if _, searching := m.modal.(modal.Search); searching {
		m = m.exitSearchMode()
	} else if m.modal != nil {
		return m
	}
	return m.enterHelpMode()
}
