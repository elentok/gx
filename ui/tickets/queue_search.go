package tickets

import (
	"strings"

	"github.com/elentok/gx/ui/search"
)

// InputFocused reports whether the search box is mid-input, so the app
// shell's digit-based tab-jump mnemonics (see ui/app's inputFocuser
// duck-type) stay routed to the search query instead of switching tabs.
func (m QueueModel) InputFocused() bool {
	return m.search.Mode() == search.SearchModeInput
}

// recomputeQueueSearchMatches rebuilds the match set against the current
// rows() order: case-insensitive substring over each ticket's title,
// mirroring the Tickets tab's recomputeSearchMatches (search.go).
func (m *QueueModel) recomputeQueueSearchMatches() {
	q := strings.ToLower(strings.TrimSpace(m.search.Query()))
	if q == "" {
		m.search.SetMatches(nil)
		return
	}

	rows := m.rows()
	matches := make([]search.Match, 0)
	for i, r := range rows {
		if strings.Contains(strings.ToLower(r.ticket.Title), q) {
			matches = append(matches, search.Match{DataIndex: i})
		}
	}
	m.search.SetMatches(matches)
}

// jumpToCurrentQueueMatch moves the selection to the search cursor's current
// match, mirroring the Tickets tab's jumpToCurrentMatch (search.go).
func (m *QueueModel) jumpToCurrentQueueMatch() {
	match, ok := m.search.Match(m.search.Cursor())
	if !ok {
		return
	}
	rows := m.rows()
	if match.DataIndex >= 0 && match.DataIndex < len(rows) {
		m.selected = match.DataIndex
		m.ensureQueueVisible()
	}
}

// queueSearchMatch reports whether the row at idx is a search match, and
// whether it's the match currently under the search cursor (n/N target).
func (m QueueModel) queueSearchMatch(idx int) (matched, current bool) {
	pos, ok := m.search.MatchPosByDataIndex(idx)
	if !ok {
		return false, false
	}
	return true, pos == m.search.Cursor()
}

func (m QueueModel) searchOverlayWidth() int {
	max := m.width * 80 / 100
	if search.DESIRED_WIDTH < max {
		return search.DESIRED_WIDTH
	}
	return max
}
