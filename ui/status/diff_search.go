package status

import (
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/filetree"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/status/diffarea"

	tea "charm.land/bubbletea/v2"
)

func (m Model) InputFocused() bool {
	if m.push.InputFocused() || m.pull.InputFocused() {
		return true
	}
	if m.focus == focusFiletree {
		return m.fileTreeModel.Search().InputFocused()
	}
	return m.currentDiffSearch().InputFocused()
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
		diffviewModel := m.diffarea.SectionModel(m.diffarea.ActiveSection)
		for _, match := range diffviewModel.ComputeSearchMatches(q) {
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

	diffviewModel := m.diffarea.SectionModel(m.diffarea.ActiveSection)
	diffviewModel.ApplySearchMatch(match)
	return m, nil

}

func (m *Model) syncSearchCursorFromDiffFocus() {
	diffSearch := m.currentDiffSearch()
	if !diffSearch.HasQuery() || diffSearch.MatchesCount() == 0 || m.focus != focusDiff {
		return
	}
	diffviewModel := m.diffarea.SectionModel(m.diffarea.ActiveSection)
	idx := diffviewModel.CurrentSearchCursor(diffSearch.Matches())
	if idx < 0 {
		return
	}

	for i := range diffSearch.Matches() {
		if i == idx {
			diffSearch.SetCursor(i)
			return
		}
	}
}

func (m *Model) currentDiffSearch() *search.Model {
	if m.diffarea.ActiveSection == diffarea.SectionStaged {
		return m.diffarea.Staged.Search()
	}
	return m.diffarea.Unstaged.Search()
}

func (m *Model) diffSearchForSection(section diffarea.Section) *search.Model {
	if section == diffarea.SectionStaged {
		return m.diffarea.Staged.Search()
	}
	return m.diffarea.Unstaged.Search()
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

func (m Model) searchMatchDiffDisplay(scope diffarea.Section, displayIdx int) (matched bool, current bool) {
	diffSearch := m.diffSearchForSection(scope)
	if !diffSearch.HasQuery() {
		return false, false
	}
	if m.focus != focusDiff || m.diffarea.ActiveSection != scope {
		return false, false
	}
	for i, match := range diffSearch.Matches() {
		if match.DisplayIndex == displayIdx {
			return true, i == diffSearch.Cursor()
		}
	}
	return false, false
}
