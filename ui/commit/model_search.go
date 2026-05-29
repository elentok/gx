package commit

import (
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/filetree"
	"github.com/elentok/gx/ui/search"
)

func (m Model) InputFocused() bool {
	return m.fileTreeModel.Search().InputFocused() || m.search.Mode() == search.SearchModeInput
}

func (m *Model) computeDiffSearchMatches(query string) []search.Match {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return []search.Match{}
	}
	var matches []search.Match
	for _, match := range m.diffModel.ComputeSearchMatches(q) {
		matches = append(matches, search.Match{Index: match.RawIndex, DisplayIndex: match.DisplayIndex})
	}
	return matches
}

// jumpToCurrentDiffMatch navigates to the match at the current diff search cursor position.
// Called synchronously after search state changes to avoid async message round-trips.
func (m *Model) jumpToCurrentDiffMatch() {
	match, ok := m.search.Match(m.search.Cursor())
	if !ok {
		return
	}
	m.focusDiff = true
	m.diffModel.FocusSearchMatch(match)
}

func (m *Model) syncSearchCursorFromDiffFocus() {
	if !m.search.HasQuery() || m.search.MatchesCount() == 0 || !m.focusDiff {
		return
	}
	idx := m.diffModel.CurrentSearchCursor(m.search.Matches())
	if idx >= 0 {
		m.search.SetCursor(idx)
	}
}

func (m Model) searchMatchDiffDisplay(displayIdx int) (matched bool, current bool) {
	if !m.search.HasQuery() {
		return false, false
	}
	for i, match := range m.search.Matches() {
		if match.DisplayIndex == displayIdx {
			return true, i == m.search.Cursor()
		}
	}
	return false, false
}

func (m Model) fileEntrySearchText(entry filetree.Entry[git.CommitFile]) string {
	if entry.Kind == filetree.EntryDir {
		return entry.DisplayName + "/"
	}
	if entry.Value.RenameFrom != "" {
		return entry.Value.RenameFrom + " -> " + entry.Value.Path
	}
	return entry.Value.Path
}

func (m Model) searchOverlayWidth() int {
	maxW := m.width * 80 / 100
	if 50 < maxW {
		return 50
	}
	return maxW
}
