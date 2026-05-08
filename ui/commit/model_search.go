package commit

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/explorer"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type commitSearchScope int

const (
	searchScopeSidebar commitSearchScope = iota
	searchScopeDiff
)

var commitSearchHighlightStyle = lipgloss.NewStyle().Foreground(ui.ColorYellow).Bold(true)
var commitSearchCurrentStyle = lipgloss.NewStyle().Foreground(ui.ColorGreen).Bold(true)

func (m Model) InputFocused() bool {
	return m.searchMode == explorer.SearchModeInput
}

func (m *Model) enterSearchMode() {
	ti := textinput.New()
	ti.Prompt = "/"
	ti.SetValue(m.searchQuery)
	ti.CursorEnd()
	ti.Focus()
	m.searchInput = ti
	m.searchMode = explorer.SearchEnter()
	m.searchScope = searchScopeSidebar
	if m.focusDiff {
		m.searchScope = searchScopeDiff
	}
}

func (m *Model) handleSearchKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if m.searchMode != explorer.SearchModeInput {
		return false, nil
	}
	total := len(m.searchMatches)
	if m.searchScope == searchScopeSidebar {
		total = len(m.fileMatches)
	}
	switch msg.String() {
	case "esc":
		mode, cleared := explorer.SearchDismiss(
			&m.searchQuery,
			&m.searchCursor,
			total,
			explorer.SearchDismissKeepResultsUnlessEmptyOrNoMatches,
		)
		m.searchMode = mode
		if cleared {
			m.clearSearch()
		}
		return true, nil
	case "enter":
		mode, cleared := explorer.SearchDismiss(
			&m.searchQuery,
			&m.searchCursor,
			total,
			explorer.SearchDismissKeepResultsUnlessEmptyOrNoMatches,
		)
		m.searchMode = mode
		if cleared {
			m.clearSearch()
		}
		return true, nil
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	m.searchQuery = m.searchInput.Value()
	m.recomputeSearchMatches()
	m.jumpToSearchCursor()
	return true, cmd
}

func (m *Model) handleSearchNavigateKey(msg tea.KeyPressMsg) bool {
	total := len(m.searchMatches)
	if m.searchScope == searchScopeSidebar {
		total = len(m.fileMatches)
	}
	if !explorer.SearchCanNavigate(m.searchQuery, total) {
		return false
	}
	switch msg.String() {
	case "n":
		explorer.SearchCursorNext(&m.searchCursor, total)
		m.jumpToSearchCursor()
		return true
	case "N", "shift+n":
		explorer.SearchCursorPrev(&m.searchCursor, total)
		m.jumpToSearchCursor()
		return true
	}
	return false
}

func (m *Model) clearSearch() {
	m.searchQuery = ""
	m.searchMatches = nil
	m.fileMatches = nil
	m.searchCursor = 0
}

func (m *Model) recomputeSearchMatches() {
	m.searchMatches = nil
	m.fileMatches = nil
	m.searchCursor = 0
	if strings.TrimSpace(m.searchQuery) == "" {
		return
	}
	if m.searchScope == searchScopeSidebar {
		q := strings.ToLower(strings.TrimSpace(m.searchQuery))
		for i, entry := range m.fileEntries {
			if strings.Contains(strings.ToLower(m.fileEntrySearchText(entry)), q) {
				m.fileMatches = append(m.fileMatches, i)
			}
		}
		return
	}
	m.searchMatches = explorer.ComputeDiffSearchMatches(m.section.ViewLines, m.section.DisplayToRaw, m.searchQuery)
}

func (m *Model) jumpToSearchCursor() {
	if m.searchScope == searchScopeSidebar {
		if len(m.fileMatches) == 0 || m.searchCursor < 0 || m.searchCursor >= len(m.fileMatches) {
			return
		}
		m.focusDiff = false
		m.selected = m.fileMatches[m.searchCursor]
		m.refreshDiff()
		return
	}
	if len(m.searchMatches) == 0 || m.searchCursor < 0 || m.searchCursor >= len(m.searchMatches) {
		return
	}
	match := m.searchMatches[m.searchCursor]
	m.focusDiff = true
	m.diffNavMode = explorer.NavLine
	explorer.ApplyDiffSearchMatch(&m.section, &m.diffViewport, match)
}

func (m Model) searchMatchDiffDisplay(displayIdx int) (matched bool, current bool) {
	if m.searchScope != searchScopeDiff {
		return false, false
	}
	if strings.TrimSpace(m.searchQuery) == "" {
		return false, false
	}
	if i := explorer.DiffSearchMatchIndex(m.searchMatches, displayIdx); i >= 0 {
		return true, i == m.searchCursor
	}
	return false, false
}

func (m Model) searchMatchSidebarIndex(idx int) (matched bool, current bool) {
	if m.searchScope != searchScopeSidebar || strings.TrimSpace(m.searchQuery) == "" {
		return false, false
	}
	for i, match := range m.fileMatches {
		if match == idx {
			return true, i == m.searchCursor
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

func (m Model) searchFooterText() string {
	if m.searchMode != explorer.SearchModeInput {
		return ""
	}
	total := len(m.searchMatches)
	if m.searchScope == searchScopeSidebar {
		total = len(m.fileMatches)
	}
	right := ""
	if strings.TrimSpace(m.searchQuery) != "" {
		if total == 0 {
			right = "no matches"
		} else {
			right = fmt.Sprintf("%d/%d", m.searchCursor+1, total)
		}
	}
	left := m.searchInput.View()
	if right == "" || m.width <= 0 {
		return left
	}
	leftW := ansi.StringWidth(left)
	rightW := ansi.StringWidth(right)
	if leftW+rightW+1 >= m.width {
		return left + " " + right
	}
	return left + strings.Repeat(" ", m.width-leftW-rightW) + right
}
