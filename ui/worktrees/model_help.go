package worktrees

import (
	"github.com/elentok/gx/ui/help"

	tea "charm.land/bubbletea/v2"
)

// helpModal wraps the help dialog. Model.helpModel is the closed template the
// sections are built into once; each open copies it.
type helpModal struct {
	model help.Model
}

func (m Model) enterHelpMode() Model {
	h := m.helpModel
	h.Open(m.width, m.height)
	return m.openModal(helpModal{model: h})
}

// handleOpenHelp answers the title row's help click. Search is the one mode
// help may replace: every other mode is a dialog the click must not cover.
func (m Model) handleOpenHelp() Model {
	if m.ModalOpen() || m.modal != nil || (m.mode != modeNormal && m.mode != modeSearch) {
		return m
	}
	if m.mode == modeSearch {
		m = m.exitSearchMode()
	}
	return m.enterHelpMode()
}

func (h helpModal) Update(msg tea.Msg) (modal, tea.Cmd, modalResult) {
	var cmd tea.Cmd
	h.model, cmd = h.model.Update(msg)
	return h, cmd, modalResult{close: !h.model.IsOpen}
}

func (h helpModal) InputFocused() bool { return h.model.InputFocused() }

func (h helpModal) View(_, _ int) string { return h.model.View() }
