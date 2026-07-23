package tickets

import (
	"strings"

	"github.com/elentok/gx/ui/search"
)

// recomputeSearchMatches rebuilds the match set against the current
// visibleRows() order: case-insensitive substring over each ticket's title
// concatenated with its rendered status word. Epic header rows never match.
func (m *Model) recomputeSearchMatches() {
	q := strings.ToLower(strings.TrimSpace(m.search.Query()))
	if q == "" {
		m.search.SetMatches(nil)
		return
	}

	rows := m.visibleRows()
	matches := make([]search.Match, 0)
	for i, r := range rows {
		if r.isEpic() {
			continue
		}
		if strings.Contains(strings.ToLower(m.searchText(r)), q) {
			matches = append(matches, search.Match{DataIndex: i})
		}
	}
	m.search.SetMatches(matches)
}

// jumpToCurrentMatch moves the selection to the search cursor's current
// match, mirroring ui/log's jumpToCurrentMatch.
func (m *Model) jumpToCurrentMatch() {
	match, ok := m.search.Match(m.search.Cursor())
	if !ok {
		return
	}
	rows := m.visibleRows()
	if match.DataIndex >= 0 && match.DataIndex < len(rows) {
		m.selected = match.DataIndex
		m.ensureSidebarVisible()
	}
}

func (m Model) searchText(r row) string {
	epic := m.epics[r.epicIdx]
	t := epic.Tickets[r.ticketIdx]
	return t.Title + " " + epic.RenderedStatus(t).Word()
}

// searchMatch reports whether the visible row at idx is a search match, and
// whether it's the match currently under the search cursor (n/N target).
func (m Model) searchMatch(idx int) (matched, current bool) {
	pos, ok := m.search.MatchPosByDataIndex(idx)
	if !ok {
		return false, false
	}
	return true, pos == m.search.Cursor()
}
