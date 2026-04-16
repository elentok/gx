package stage

import (
	"fmt"
	"strings"

	"gx/ui"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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

var stageSearchHighlightStyle = lipgloss.NewStyle().Foreground(catYellow).Bold(true)

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
		for i := 0; i < len(sec.viewLines) && i < len(sec.displayToRaw); i++ {
			line := strings.ToLower(ansi.Strip(sec.viewLines[i]))
			if strings.Contains(line, q) {
				m.searchMatches = append(m.searchMatches, stageSearchMatch{displayIndex: i, rawIndex: sec.displayToRaw[i], scope: scope})
			}
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
	if match.displayIndex >= 0 {
		if match.displayIndex < sec.viewport.YOffset() {
			sec.viewport.SetYOffset(match.displayIndex)
		} else {
			last := sec.viewport.YOffset() + sec.viewport.VisibleLineCount() - 1
			if sec.viewport.VisibleLineCount() > 0 && match.displayIndex > last {
				sec.viewport.SetYOffset(maxInt(0, match.displayIndex-sec.viewport.VisibleLineCount()+1))
			}
		}
	}
	if match.rawIndex >= 0 {
		for i, ch := range sec.parsed.Changed {
			if ch.LineIndex == match.rawIndex {
				sec.activeLine = i
				break
			}
		}
		for i, h := range sec.parsed.Hunks {
			if match.rawIndex >= h.StartLine && match.rawIndex <= h.EndLine {
				sec.activeHunk = i
				break
			}
		}
	}
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
	if m.navMode != navLine || sec.activeLine < 0 || sec.activeLine >= len(sec.parsed.Changed) {
		return
	}
	raw := sec.parsed.Changed[sec.activeLine].LineIndex
	for i, match := range m.searchMatches {
		if match.scope == expected && match.rawIndex == raw {
			m.searchCursor = i
			return
		}
	}
}

func (m *Model) searchScopeSection(scope stageSearchScope) *sectionState {
	if scope == searchScopeStaged {
		return &m.staged
	}
	return &m.unstaged
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

func (m Model) searchFooterText() string {
	if m.searchMode == searchModeNone {
		return ""
	}
	total := len(m.searchMatches)
	idx := 0
	if total > 0 {
		idx = m.searchCursor + 1
	}
	base := fmt.Sprintf("search: %s", m.searchInput.View())
	if strings.TrimSpace(m.searchQuery) != "" {
		if total == 0 {
			base += " · no matches"
		} else {
			base += fmt.Sprintf(" · %d/%d", idx, total)
		}
	}
	base = ui.JoinStatus(
		base,
		ui.RenderInlineBindings(
			key.NewBinding(key.WithHelp("enter", "keep highlights")),
			key.NewBinding(key.WithHelp("esc", "clear")),
		),
	)
	return base
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

func highlightMatchText(text, query string) string {
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
	return text[:idx] + stageSearchHighlightStyle.Render(text[idx:end]) + text[end:]
}
