package tickets

import (
	"strings"

	"github.com/elentok/gx/ui/search"
)

// ticketRef identifies a ticket by its epic and position within that epic's
// Tickets slice, independent of visibleRows()/collapse state.
type ticketRef struct {
	epicIdx   int
	ticketIdx int
}

// recomputeSearchMatches matches against the underlying epic/ticket data
// directly (m.epics), not visibleRows(), so a match inside a collapsed epic
// (every closed epic starts collapsed, see defaultCollapsedEpics) is still
// found: case-insensitive substring over each ticket's title concatenated
// with its rendered status word. Any epic containing a match is auto-expanded
// first so the match's eventual DataIndex (looked up post-expansion) lands on
// a row that's actually rendered. Epic header rows never match. A query
// matching nothing leaves collapse state untouched.
func (m *Model) recomputeSearchMatches() {
	q := strings.ToLower(strings.TrimSpace(m.search.Query()))
	if q == "" {
		m.search.SetMatches(nil)
		return
	}

	var matchedRefs []ticketRef
	for epicIdx, epic := range m.epics {
		for ticketIdx, t := range epic.Tickets {
			text := strings.ToLower(t.Title + " " + epic.RenderedStatus(t).Word())
			if strings.Contains(text, q) {
				matchedRefs = append(matchedRefs, ticketRef{epicIdx: epicIdx, ticketIdx: ticketIdx})
			}
		}
	}

	if len(matchedRefs) == 0 {
		m.search.SetMatches(nil)
		return
	}

	if m.collapsedEpics == nil {
		m.collapsedEpics = map[string]bool{}
	}
	wanted := make(map[ticketRef]bool, len(matchedRefs))
	for _, ref := range matchedRefs {
		wanted[ref] = true
		m.collapsedEpics[m.epics[ref.epicIdx].Path] = false
	}

	matches := make([]search.Match, 0, len(matchedRefs))
	for i, r := range m.visibleRows() {
		if r.isEpic() {
			continue
		}
		if wanted[ticketRef{epicIdx: r.epicIdx, ticketIdx: r.ticketIdx}] {
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

// searchMatch reports whether the visible row at idx is a search match, and
// whether it's the match currently under the search cursor (n/N target).
func (m Model) searchMatch(idx int) (matched, current bool) {
	pos, ok := m.search.MatchPosByDataIndex(idx)
	if !ok {
		return false, false
	}
	return true, pos == m.search.Cursor()
}
