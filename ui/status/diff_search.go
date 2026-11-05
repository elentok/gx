package status

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/status/diffarea"
)

func (m Model) InputFocused() bool {
	return m.git.InputFocused() || m.help.InputFocused() ||
		m.searchInputFocused()
}

func (m Model) searchInputFocused() bool {
	if m.focus == focusFiletree {
		return m.filetree.Model.Search().InputFocused()
	}
	return m.diffarea.ActiveSectionModel().Search().InputFocused()
}

func (m Model) ModalOpen() bool {
	return m.modals.Open() ||
		m.git.IsOpen() || m.help.IsOpen
}

func (m Model) handleJumpToMatch(msg search.JumpToMatchMsg) (Model, tea.Cmd) {
	match := msg.Match
	if m.focus == focusFiletree {
		if match.DataIndex >= 0 && match.DataIndex < len(m.filetree.Model.Entries()) {
			m.filetree.SetSelection(match.DataIndex)
			onFiletreeSelectionChanged(&m)
			return m, m.diffLoader.scheduleDiffReload()
		}
	}
	return m, nil
}

func (m Model) searchCounterForFiletreePane() string {
	search := m.filetree.Model.Search()
	if m.focus != focusFiletree || !search.HasQuery() || search.MatchesCount() == 0 {
		return ""
	}
	return diffarea.SearchCounterText(m.settings.UseNerdFontIcons, search.Cursor(), search.MatchesCount())
}
