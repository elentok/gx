package status

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/explorer"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type stageSearchMode int

const (
	searchModeNone stageSearchMode = iota
	searchModeInput
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

var stageSearchHighlightStyle = lipgloss.NewStyle().Foreground(catYellow).Bold(true).Underline(true)
var stageSearchCurrentStyle = lipgloss.NewStyle().Foreground(catGreen).Bold(true).Underline(true)

func (m *Model) enterSearchMode() {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Focus()
	m.searchInput = ti
	m.searchMode = searchModeInput
	m.searchScope = m.currentSearchScope()
	m.searchQuery = ""
	m.searchMatches = nil
	m.searchCursor = 0
}

func (m *Model) exitSearchMode() {
	m.searchMode = searchModeNone
	m.searchQuery = ""
	m.searchMatches = nil
	m.searchCursor = 0
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

func (m Model) handleSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.searchMode {
	case searchModeInput:
		switch msg.String() {
		case "esc":
			m.exitSearchMode()
			return m, nil
		case "enter":
			if m.focus == focusDiff {
				m.navMode = navLine
			}
			m.searchMode = searchModeNone
			return m, nil
		}

		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.searchQuery = m.searchInput.Value()
		jumpCmd := m.recomputeSearchMatchesAndJumpToFirst()
		if jumpCmd != nil {
			return m, jumpCmd
		}
		return m, cmd

	}
	return m, nil
}

func (m *Model) handleSearchNavigateKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if strings.TrimSpace(m.searchQuery) == "" || len(m.searchMatches) == 0 {
		return nil, false
	}
	switch msg.String() {
	case "n":
		if m.searchCursor < len(m.searchMatches)-1 {
			m.searchCursor++
		}
		return m.jumpToSearchCursor(), true
	case "N", "shift+n":
		if m.searchCursor > 0 {
			m.searchCursor--
		}
		return m.jumpToSearchCursor(), true
	}
	return nil, false
}

func (m *Model) recomputeSearchMatchesAndJumpToFirst() tea.Cmd {
	m.recomputeSearchMatches()
	if len(m.searchMatches) == 0 {
		return nil
	}
	return m.jumpToSearchCursor()
}

func (m *Model) recomputeSearchMatches() {
	q := strings.ToLower(strings.TrimSpace(m.searchQuery))
	m.searchMatches = nil
	m.searchCursor = 0
	if q == "" {
		return
	}

	scope := m.searchScope
	if scope == searchScopeStatus {
		for i, entry := range m.statusEntries {
			text := strings.ToLower(m.statusEntrySearchText(entry))
			if strings.Contains(text, q) {
				m.searchMatches = append(m.searchMatches, stageSearchMatch{statusIndex: i, scope: scope})
			}
		}
	} else {
		sec := m.searchScopeSection(scope)
		for _, match := range explorer.ComputeDiffSearchMatches(sec.viewLines, sec.displayToRaw, q) {
			m.searchMatches = append(m.searchMatches, stageSearchMatch{
				displayIndex: match.DisplayIndex,
				rawIndex:     match.RawIndex,
				scope:        scope,
			})
		}
	}
}

func (m *Model) jumpToSearchCursor() tea.Cmd {
	if len(m.searchMatches) == 0 || m.searchCursor < 0 || m.searchCursor >= len(m.searchMatches) {
		return nil
	}
	match := m.searchMatches[m.searchCursor]
	if match.scope == searchScopeStatus {
		if match.statusIndex >= 0 && match.statusIndex < len(m.statusEntries) {
			m.selected = match.statusIndex
			m.onStatusSelectionChanged()
			return m.scheduleDiffReload()
		}
		return nil
	}

	sec := m.searchScopeSection(match.scope)
	data := toExplorerSectionData(*sec)
	explorer.ApplyDiffSearchMatch(&data, &sec.viewport, explorer.DiffSearchMatch{
		DisplayIndex: match.displayIndex,
		RawIndex:     match.rawIndex,
	})
	*sec = fromExplorerSectionData(data, sec.viewport)
	return nil
}

func (m *Model) syncSearchCursorFromDiffFocus() {
	if strings.TrimSpace(m.searchQuery) == "" || len(m.searchMatches) == 0 || m.focus != focusDiff {
		return
	}
	expected := searchScopeUnstaged
	if m.section == sectionStaged {
		expected = searchScopeStaged
	}
	if m.searchScope != expected {
		return
	}
	sec := m.searchScopeSection(expected)
	data := toExplorerSectionData(*sec)
	diffMatches := make([]explorer.DiffSearchMatch, 0, len(m.searchMatches))
	for _, match := range m.searchMatches {
		if match.scope == expected {
			diffMatches = append(diffMatches, explorer.DiffSearchMatch{
				DisplayIndex: match.displayIndex,
				RawIndex:     match.rawIndex,
			})
		}
	}
	idx := explorer.CurrentDiffSearchMatchIndex(data, diffMatches, explorer.NavLine)
	if idx < 0 {
		return
	}
	cursor := 0
	for i, match := range m.searchMatches {
		if match.scope != expected {
			continue
		}
		if cursor == idx {
			m.searchCursor = i
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

func (m Model) searchInputOverlayView() string {
	outerW := m.searchOverlayWidth()
	innerW := outerW - 2 - 2
	ti := m.searchInput
	ti.SetWidth(innerW)

	var rightTitle string
	total := len(m.searchMatches)
	if strings.TrimSpace(m.searchQuery) != "" {
		if total == 0 {
			rightTitle = "no matches"
		} else {
			rightTitle = fmt.Sprintf("%d/%d", m.searchCursor+1, total)
		}
	}

	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:         "Search",
		RightTitle:    rightTitle,
		Body:          ti.View(),
		Width:         outerW,
		BorderColor:   ui.ColorBorder,
		TitleColor:    ui.ColorBlue,
		TitleInBorder: true,
	})
}

func (m Model) searchMatchStatusIndex(idx int) bool {
	if m.searchScope != searchScopeStatus || strings.TrimSpace(m.searchQuery) == "" {
		return false
	}
	for _, match := range m.searchMatches {
		if match.scope == searchScopeStatus && match.statusIndex == idx {
			return true
		}
	}
	return false
}

func (m Model) searchMatchDiffDisplay(scope diffSection, displayIdx int) (matched bool, current bool) {
	if strings.TrimSpace(m.searchQuery) == "" {
		return false, false
	}
	expected := searchScopeUnstaged
	if scope == sectionStaged {
		expected = searchScopeStaged
	}
	if m.searchScope != expected {
		return false, false
	}
	for i, match := range m.searchMatches {
		if match.scope == expected && match.displayIndex == displayIdx {
			return true, i == m.searchCursor
		}
	}
	return false, false
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
	style := stageSearchHighlightStyle
	if current {
		style = stageSearchCurrentStyle
	}
	return text[:idx] + style.Render(text[idx:end]) + text[end:]
}
