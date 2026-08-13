package tickets

import (
	"strings"

	"github.com/elentok/gx/ui/search"
)

// InputFocused reports whether the search box is mid-input, so the app
// shell's digit-based tab-jump mnemonics (see ui/app's inputFocuser
// duck-type) stay routed to the search query instead of switching tabs.
func (m QueueModel) InputFocused() bool {
	return m.help.InputFocused() || m.search.Mode() == search.SearchModeInput
}

// ModalOpen reports whether one of the tab's dialogs is open, so the app
// shell (see ui/app's modalOpener duck-type) blocks tab-switch keys and
// routes them here instead while it's up.
func (m QueueModel) ModalOpen() bool {
	return m.help.IsOpen || m.implementAgentMenuOpen || m.confirm.IsOpen
}

// recomputeQueueSearchMatches rebuilds the match set against the current
// m.queueTree.Entries() order: case-insensitive substring over each ticket
// entry's title, mirroring the Tickets tab's recomputeSearchMatches
// (search.go). Header/separator/error entries never match.
func (m *QueueModel) recomputeQueueSearchMatches() {
	q := strings.ToLower(strings.TrimSpace(m.search.Query()))
	if q == "" {
		m.search.SetMatches(nil)
		return
	}

	matches := make([]search.Match, 0)
	for i, e := range m.queueTree.Entries() {
		if e.Value.kind != nodeQueueTicket {
			continue
		}
		if strings.Contains(strings.ToLower(e.Value.ticket.ticket.Title), q) {
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
	if match.DataIndex >= 0 && match.DataIndex < len(m.queueTree.Entries()) {
		m.queueTree.SetSelectedIndex(match.DataIndex)
	}
}

// queueSearchMatch reports whether the row at idx is a search match, and
// whether it's the match currently under the search cursor (n/N target).
func (m QueueModel) queueSearchMatch(idx int) (matched, current bool) {
	return m.queueTree.SearchMatch(idx)
}

func (m QueueModel) searchOverlayWidth() int {
	max := m.width * 80 / 100
	if search.DESIRED_WIDTH < max {
		return search.DESIRED_WIDTH
	}
	return max
}
