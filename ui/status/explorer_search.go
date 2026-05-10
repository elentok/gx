package status

import (
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/explorer"
	"github.com/elentok/gx/ui/filetree"
	"github.com/elentok/gx/ui/search"

	tea "charm.land/bubbletea/v2"
)

func (m Model) InputFocused() bool {
	if m.focus == focusFiletree {
		return m.fileTreeModel.Search().Mode() == search.SearchModeInput
	}
	return m.currentDiffSearch().Mode() == search.SearchModeInput
}

func (m *Model) computeSearchMatches(query string) []search.Match {
	q := strings.ToLower(strings.TrimSpace(query))

	if q == "" {
		return []search.Match{}
	}

	var matches []search.Match

	if m.focus == focusFiletree {
		for i, entry := range m.fileTreeModel.Entries() {
			text := strings.ToLower(m.filetreeEntrySearchText(entry))
			if strings.Contains(text, q) {
				matches = append(matches, search.Match{Index: i})
			}
		}
	} else {
		sec := m.sectionState(m.section)
		for _, match := range explorer.ComputeDiffSearchMatches(sec.data.ViewLines, sec.data.DisplayToRaw, q) {
			matches = append(matches, search.Match{
				DisplayIndex: match.DisplayIndex,
				Index:        match.RawIndex,
			})
		}
	}

	return matches
}

func (m *Model) recomputeSearchMatches() {
	diffSearch := m.currentDiffSearch()
	matches := m.computeSearchMatches(diffSearch.Query())
	diffSearch.SetMatches(matches)
}

func (m *Model) diffSearchActiveInFocus() bool {
	return m.focus == focusDiff && m.currentDiffSearch().HasQuery()
}

func (m Model) handleJumpToMatch(msg search.JumpToMatchMsg) (Model, tea.Cmd) {
	match := msg.Match
	if m.focus == focusFiletree {
		if match.Index >= 0 && match.Index < len(m.fileTreeModel.Entries()) {
			m.setStatusSelection(match.Index)
			m.onFiletreeSelectionChanged()
			return m, m.scheduleDiffReload()
		}
		return m, nil
	}

	sec := m.sectionState(m.section)
	explorer.ApplyDiffSearchMatch(&sec.data, &sec.viewport, match)
	return m, nil

}

func (m *Model) syncSearchCursorFromDiffFocus() {
	diffSearch := m.currentDiffSearch()
	if !diffSearch.HasQuery() || diffSearch.MatchesCount() == 0 || m.focus != focusDiff {
		return
	}
	sec := m.sectionState(m.section)
	diffMatches := make([]explorer.DiffSearchMatch, 0, diffSearch.MatchesCount())
	for _, match := range diffSearch.Matches() {
		diffMatches = append(diffMatches, explorer.DiffSearchMatch{
			DisplayIndex: match.DisplayIndex,
			RawIndex:     match.Index,
		})
	}
	idx := explorer.CurrentDiffSearchMatchIndex(sec.data, diffMatches, explorer.NavLine)
	if idx < 0 {
		return
	}

	// TODO: check if this is working
	cursor := 0
	for i := range diffSearch.Matches() {
		if cursor == idx {
			diffSearch.SetCursor(i)
			return
		}
		cursor++
	}
}

func (m *Model) currentDiffSearch() *search.Model {
	if m.section == sectionStaged {
		return m.stagedDiffModel.Search()
	}
	return m.unstagedDiffModel.Search()
}

func (m *Model) diffSearchForSection(section diffSection) *search.Model {
	if section == sectionStaged {
		return m.stagedDiffModel.Search()
	}
	return m.unstagedDiffModel.Search()
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

func (m Model) searchMatchStatusIndex(idx int) bool {
	search := m.fileTreeModel.Search()
	if m.focus != focusFiletree || !search.HasQuery() {
		return false
	}
	for _, match := range search.Matches() {
		if match.Index == idx {
			return true
		}
	}
	return false
}

func (m Model) searchMatchDiffDisplay(scope diffSection, displayIdx int) (matched bool, current bool) {
	diffSearch := m.diffSearchForSection(scope)
	if !diffSearch.HasQuery() {
		return false, false
	}
	if m.focus != focusDiff || m.section != scope {
		return false, false
	}
	for i, match := range diffSearch.Matches() {
		if match.DisplayIndex == displayIdx {
			return true, i == diffSearch.Cursor()
		}
	}
	return false, false
}
