package status

import (
	"strings"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/explorer"
	"github.com/elentok/gx/ui/search"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type stageSearchScope int

const (
	searchScopeStatus stageSearchScope = iota
	searchScopeUnstaged
	searchScopeStaged
)

type stageSearchMatch struct {
	statusIndex  int
	displayIndex int
	rawIndex     int
	scope        stageSearchScope
}

var stageSearchHighlightStyle = lipgloss.NewStyle().Foreground(ui.ColorYellow).Bold(true).Underline(true)
var stageSearchCurrentStyle = lipgloss.NewStyle().Foreground(ui.ColorGreen).Bold(true).Underline(true)

func (m Model) InputFocused() bool {
	return m.search.Mode() == search.SearchModeInput
}

func (m Model) currentSearchScope() stageSearchScope {
	if m.focus == focusStatus {
		return searchScopeStatus
	}
	if m.section == sectionStaged {
		return searchScopeStaged
	}
	return searchScopeUnstaged
}

func (m *Model) computeSearchMatches(query string) []search.Match {
	q := strings.ToLower(strings.TrimSpace(query))

	if q == "" {
		return []search.Match{}
	}

	var matches []search.Match

	scope := m.currentSearchScope()
	if scope == searchScopeStatus {
		for i, entry := range m.statusEntries {
			text := strings.ToLower(m.statusEntrySearchText(entry))
			if strings.Contains(text, q) {
				matches = append(matches, search.Match{Index: i})
			}
		}
	} else {
		sec := m.searchScopeSection(scope)
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
	matches := m.computeSearchMatches(m.search.Query())
	m.search.SetMatches(matches)
}

func (m Model) handleJumpToMatch(msg search.JumpToMatchMsg) (Model, tea.Cmd) {
	match := msg.Match
	if m.currentSearchScope() == searchScopeStatus {
		if match.Index >= 0 && match.Index < len(m.statusEntries) {
			m.setStatusSelection(match.Index)
			m.onStatusSelectionChanged()
			return m, m.scheduleDiffReload()
		}
		return m, nil
	}

	sec := m.searchScopeSection(m.currentSearchScope())
	explorer.ApplyDiffSearchMatch(&sec.data, &sec.viewport, match)
	return m, nil

}

func (m *Model) syncSearchCursorFromDiffFocus() {
	if !m.search.HasQuery() || m.search.MatchesCount() == 0 || m.focus != focusDiff {
		return
	}
	expected := searchScopeUnstaged
	if m.section == sectionStaged {
		expected = searchScopeStaged
	}
	if m.currentSearchScope() != expected {
		return
	}
	sec := m.searchScopeSection(expected)
	diffMatches := make([]explorer.DiffSearchMatch, 0, m.search.MatchesCount())
	for _, match := range m.search.Matches() {
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
	for i, _ := range m.search.Matches() {
		if cursor == idx {
			m.search.SetCursor(i)
			return
		}
		cursor++
	}
}

func (m *Model) searchScopeSection(scope stageSearchScope) *sectionState {
	if scope == searchScopeStaged {
		return m.sectionState(sectionStaged)
	}
	return m.sectionState(sectionUnstaged)
}

func (m Model) statusEntrySearchText(entry statusEntry) string {
	name := entry.DisplayName
	if entry.Kind == statusEntryFile && entry.File.IsRenamed() && entry.File.RenameFrom != "" {
		name = entry.File.RenameFrom + " -> " + entry.File.Path
	}
	if entry.Kind == statusEntryDir {
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
	if m.currentSearchScope() != searchScopeStatus || !m.search.HasQuery() {
		return false
	}
	for _, match := range m.search.Matches() {
		if match.Index == idx {
			return true
		}
	}
	return false
}

func (m Model) searchMatchDiffDisplay(scope diffSection, displayIdx int) (matched bool, current bool) {
	if !m.search.HasQuery() {
		return false, false
	}
	expected := searchScopeUnstaged
	if scope == sectionStaged {
		expected = searchScopeStaged
	}
	if m.currentSearchScope() != expected {
		return false, false
	}
	for i, match := range m.search.Matches() {
		if match.DisplayIndex == displayIdx {
			return true, i == m.search.Cursor()
		}
	}
	return false, false
}
