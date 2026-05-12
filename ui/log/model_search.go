package log

import (
	"strings"

	"github.com/elentok/gx/ui/search"
)

func (m *Model) jumpToCurrentMatch() {
	match, ok := m.search.Match(m.search.Cursor())
	if !ok {
		return
	}
	if match.Index >= 0 && match.Index < len(m.rows) {
		m.cursor = match.Index
	}
}

func (m *Model) recomputeSearchMatches() {
	q := strings.ToLower(strings.TrimSpace(m.search.Query()))
	if q == "" {
		m.search.SetMatches(nil)
		return
	}

	matches := make([]search.Match, 0)
	for i, row := range m.rows {
		if strings.Contains(strings.ToLower(m.searchText(row)), q) {
			matches = append(matches, search.Match{Index: i})
		}
	}
	m.search.SetMatches(matches)
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
