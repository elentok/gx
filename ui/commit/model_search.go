package commit

import (
	"strings"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/search"

	"charm.land/lipgloss/v2"
)

type commitSearchScope int

const (
	searchScopeSidebar commitSearchScope = iota
	searchScopeDiff
)

var commitSearchHighlightStyle = lipgloss.NewStyle().Foreground(ui.ColorYellow).Bold(true)
var commitSearchCurrentStyle = lipgloss.NewStyle().Foreground(ui.ColorGreen).Bold(true)

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
		for i, entry := range m.fileEntries {
			if strings.Contains(strings.ToLower(m.fileEntrySearchText(entry)), q) {
				matches = append(matches, search.Match{Index: i})
			}
		}
		return matches
	}
	var matches []search.Match
	for _, match := range diffview.ComputeDiffSearchMatches(m.section.ViewLines, m.section.DisplayToRaw, q) {
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
		if match.Index >= 0 && match.Index < len(m.fileEntries) {
			m.focusDiff = false
			m.selected = match.Index
			m.refreshDiff()
		}
		return
	}
	m.focusDiff = true
	m.diffNavMode = diffview.NavModeLine
	diffview.ApplyDiffSearchMatch(&m.section, &m.diffViewport, match)
}

func (m *Model) syncSearchCursorFromDiffFocus() {
	if !m.search.HasQuery() || m.search.MatchesCount() == 0 || !m.focusDiff {
		return
	}
	diffMatches := make([]diffview.DiffSearchMatch, 0, m.search.MatchesCount())
	for _, match := range m.search.Matches() {
		diffMatches = append(diffMatches, diffview.DiffSearchMatch{
			DisplayIndex: match.DisplayIndex,
			RawIndex:     match.Index,
		})
	}
	idx := diffview.CurrentDiffSearchMatchIndex(m.section, diffMatches, diffview.NavModeLine)
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

func (m Model) fileEntrySearchText(entry commitFileEntry) string {
	if entry.Kind == commitFileEntryDir {
		return entry.DisplayName + "/"
	}
	if entry.File.RenameFrom != "" {
		return entry.File.RenameFrom + " -> " + entry.File.Path
	}
	return entry.File.Path
}

func highlightMatchText(text, query string, current bool) string {
	if strings.TrimSpace(query) == "" {
		return text
	}
	lower := strings.ToLower(text)
	lq := strings.ToLower(query)
	idx := strings.Index(lower, lq)
	if idx < 0 {
		return text
	}
	end := idx + len(query)
	if end > len(text) {
		end = len(text)
	}
	style := commitSearchHighlightStyle
	if current {
		style = commitSearchCurrentStyle
	}
	return text[:idx] + style.Render(text[idx:end]) + text[end:]
}

func (m Model) searchOverlayWidth() int {
	maxW := m.width * 80 / 100
	if 50 < maxW {
		return 50
	}
	return maxW
}
