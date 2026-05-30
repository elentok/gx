package status

import (
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/filetree"
	"github.com/elentok/gx/ui/search"
	tea "charm.land/bubbletea/v2"
)

func (m Model) InputFocused() bool {
	if m.push.InputFocused() || m.pull.InputFocused() {
		return true
	}
	if m.focus == focusFiletree {
		return m.fileTreeModel.Search().InputFocused()
	}
	return m.diffarea.ActiveSectionModel().Search().InputFocused()
}

func (m *Model) computeSearchMatches(query string) []search.Match {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return []search.Match{}
	}
	var matches []search.Match
	for i, entry := range m.fileTreeModel.Entries() {
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
		if match.DataIndex >= 0 && match.DataIndex < len(m.fileTreeModel.Entries()) {
			m.setStatusSelection(match.DataIndex)
			m.onFiletreeSelectionChanged()
			return m, m.scheduleDiffReload()
		}
	}
	return m, nil
}

func (m *Model) syncSearchToInactivePane() {
	active := m.diffarea.ActiveSectionModel()
	inactive := m.diffarea.InactiveSectionModel()
	query := active.Search().Query()
	diffMatches := inactive.ComputeSearchMatches(query)
	searchMatches := make([]search.Match, len(diffMatches))
	for i, dm := range diffMatches {
		searchMatches[i] = search.Match{DataIndex: dm.RawIndex, ViewportRow: dm.DisplayIndex}
	}
	inactive.Search().Start(query)
	inactive.Search().SetMatches(searchMatches)
	inactive.Search().DismissAndKeepResults()
}

func (m Model) filetreeEntrySearchText(entry filetree.Entry[git.StageFileStatus]) string {
	name := entry.DisplayName
	if entry.Kind == filetree.EntryFile && entry.Value.IsRenamed() && entry.Value.RenameFrom != "" {
		name = entry.Value.RenameFrom + " -> " + entry.Path
	}
	if entry.Kind == filetree.EntryDir {
		return name + "/"
	}
	return name
}

const searchOverlayDesiredWidth = 50

func (m Model) searchOverlayWidth() int {
	max := m.width * 80 / 100
	if searchOverlayDesiredWidth < max {
		return searchOverlayDesiredWidth
	}
	return max
}
