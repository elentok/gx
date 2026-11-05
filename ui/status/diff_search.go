package status

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/status/diffarea"
	"github.com/elentok/gx/ui/tree"
)

func (m Model) InputFocused() bool {
	return m.git.InputFocused() || m.help.InputFocused() ||
		m.searchInputFocused()
}

func (m Model) searchInputFocused() bool {
	if m.focus == focusFiletree {
		return m.filetree.model.Search().InputFocused()
	}
	return m.diffarea.ActiveSectionModel().Search().InputFocused()
}

func (m Model) ModalOpen() bool {
	return m.modals.Open() ||
		m.git.IsOpen() || m.help.IsOpen
}

func (m *Model) computeSearchMatches(query string) []search.Match {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return []search.Match{}
	}
	var matches []search.Match
	for i, entry := range m.filetree.model.Entries() {
		text := strings.ToLower(m.filetreeEntrySearchText(entry))
		if strings.Contains(text, q) {
			matches = append(matches, search.Match{DataIndex: i})
		}
	}
	return matches
}

func (m Model) handleJumpToMatch(msg search.JumpToMatchMsg) (Model, tea.Cmd) {
	match := msg.Match
	if m.focus == focusFiletree {
		if match.DataIndex >= 0 && match.DataIndex < len(m.filetree.model.Entries()) {
			m.filetree.setSelection(match.DataIndex)
			m.onFiletreeSelectionChanged()
			return m, m.diffLoader.scheduleDiffReload()
		}
	}
	return m, nil
}

func (m Model) searchCounterForFiletreePane() string {
	search := m.filetree.model.Search()
	if m.focus != focusFiletree || !search.HasQuery() || search.MatchesCount() == 0 {
		return ""
	}
	return diffarea.SearchCounterText(m.settings.UseNerdFontIcons, search.Cursor(), search.MatchesCount())
}

func (m Model) filetreeEntrySearchText(entry tree.Entry[stage.StageFileStatus]) string {
	name := entry.DisplayName
	if !entry.HasChildren && entry.Value.IsRenamed() && entry.Value.RenameFrom != "" {
		name = entry.Value.RenameFrom + " -> " + entry.ID
	}
	if entry.HasChildren {
		return name + "/"
	}
	return name
}
