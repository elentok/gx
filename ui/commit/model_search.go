package commit

import (
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/filetree"
	"github.com/elentok/gx/ui/search"
)

type commitSearchScope int

const (
	searchScopeSidebar commitSearchScope = iota
	searchScopeDiff
)

func (m Model) InputFocused() bool {
	return m.search.Mode() == search.SearchModeInput
}

func (m *Model) computeSearchMatches(query string) []search.Match {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return []search.Match{}
	}
	if m.searchScope == searchScopeSidebar {
		var matches []search.Match
		for i, entry := range m.fileTreeModel.Entries() {
			if strings.Contains(strings.ToLower(m.fileEntrySearchText(entry)), q) {
				matches = append(matches, search.Match{Index: i})
			}
		}
		return matches
	}
	var matches []search.Match
	for _, match := range m.diffModel.ComputeSearchMatches(q) {
		matches = append(matches, search.Match{Index: match.RawIndex, DisplayIndex: match.DisplayIndex})
	}
	return matches
}

// jumpToCurrentMatch navigates to the match at the current search cursor position.
// Called synchronously after search state changes to avoid async message round-trips.
func (m *Model) jumpToCurrentMatch() {
	match, ok := m.search.Match(m.search.Cursor())
	if !ok {
		return
	}
	if m.searchScope == searchScopeSidebar {
		if match.Index >= 0 && match.Index < len(m.fileTreeModel.Entries()) {
			m.focusDiff = false
			m.fileTreeModel.SetSelectedIndex(match.Index)
			m.refreshDiff()
		}
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
	if m.searchScope != searchScopeDiff || !m.search.HasQuery() {
		return false, false
	}
	for i, match := range m.search.Matches() {
		if match.DisplayIndex == displayIdx {
			return true, i == m.search.Cursor()
		}
	}
	return false, false
}

func (m Model) searchMatchSidebarIndex(idx int) (matched bool, current bool) {
	if m.searchScope != searchScopeSidebar || !m.search.HasQuery() {
		return false, false
	}
	for i, match := range m.search.Matches() {
		if match.Index == idx {
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
