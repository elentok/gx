package log

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

func (m *Model) enterSearchMode() {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Focus()
	m.searchInput = ti
	m.searchMode = searchModeInput
	m.searchQuery = ""
	m.searchMatch = nil
	m.searchCursor = 0
}

func (m Model) handleSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, logKeySearchClose):
		m.closeSearch()
		return m, nil
	case key.Matches(msg, logKeySearchNext):
		m.advanceSearch(1)
		return m, nil
	case key.Matches(msg, logKeySearchPrev):
		m.advanceSearch(-1)
		return m, nil
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	m.searchQuery = m.searchInput.Value()
	m.recomputeSearchMatches()
	if len(m.searchMatch) > 0 {
		m.searchCursor = 0
		m.cursor = m.searchMatch[0]
	}
	return m, cmd
}

func (m *Model) closeSearch() {
	m.searchMode = searchModeNone
	if strings.TrimSpace(m.searchQuery) == "" || len(m.searchMatch) == 0 {
		m.searchQuery = ""
		m.searchMatch = nil
		m.searchCursor = 0
	}
}

func (m *Model) advanceSearch(delta int) {
	if len(m.searchMatch) == 0 {
		return
	}
	m.searchCursor = (m.searchCursor + delta + len(m.searchMatch)) % len(m.searchMatch)
	m.cursor = m.searchMatch[m.searchCursor]
}

func (m *Model) recomputeSearchMatches() {
	q := strings.ToLower(strings.TrimSpace(m.searchQuery))
	m.searchMatch = nil
	if q == "" {
		return
	}
	for i, row := range m.rows {
		if strings.Contains(strings.ToLower(m.searchText(row)), q) {
			m.searchMatch = append(m.searchMatch, i)
		}
	}
}

func (m Model) searchText(row row) string {
	if row.kind == rowPseudoStatus {
		return row.label + " " + row.detail
	}
	parts := []string{row.commit.Hash, row.commit.Subject, row.commit.AuthorName}
	for _, decoration := range row.commit.Decorations {
		parts = append(parts, decoration.Name)
	}
	return strings.Join(parts, " ")
}
