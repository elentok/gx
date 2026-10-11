package tickets

import (
	"strings"

	"github.com/elentok/gx/ui/search"
)

// ticketRef identifies a ticket by its epic and position within that epic's
// Tickets slice, independent of m.sidebarTree's entries/collapse state.
type ticketRef struct {
	epicIdx   int
	ticketIdx int
}

// recomputeSearchMatches matches against the underlying epic/ticket data
// directly (m.epics), not m.sidebarTree.Entries(), so a match inside a
// collapsed epic or a collapsed Closed section (see deriveCollapsedSidebar)
// is still found: ticketMatchesQuery for ticket rows, epicMatchesQuery for
// epic rows. Any epic containing a match is auto-expanded first, along with
// its containing section (via m.sidebarTree's collapsed IDs), so the match's
// eventual DataIndex (looked up post-expansion, post-rebuild) lands on an
// entry that's actually rendered. Section header rows never match. A query
// matching nothing leaves collapse state untouched.
func (m *Model) recomputeSearchMatches() {
	q := strings.ToLower(strings.TrimSpace(m.search.Query()))
	if q == "" {
		m.search.SetMatches(nil)
		// m.sidebarTree's own internal search.Model backs SearchMatch's
		// per-row lookup (view.go's dim/highlight): its HasQuery() gates
		// SearchMatch, so the query has to land there too, not just on the
		// matches — SetPassiveResults sets both in one call.
		m.sidebarTree.Search().SetPassiveResults("", nil)
		// Re-derive collapse state now (rather than waiting for the next
		// unrelated rebuild) so ending a search drops its transient
		// auto-expand override immediately, per ticket 02.
		m.refreshSidebarCollapse()
		m.sidebarTree.SetEntries(m.buildSidebarEntries())
		return
	}

	// An epic row is keyed as ticketRef{epicIdx, -1}, matching its
	// sidebarNode's ticketIdx.
	wanted := map[ticketRef]bool{}
	for epicIdx, epic := range m.epics {
		if epicMatchesQuery(epic, q) {
			wanted[ticketRef{epicIdx: epicIdx, ticketIdx: -1}] = true
		}
		for ticketIdx, t := range epic.Tickets {
			if ticketMatchesQuery(epic, t, q) {
				wanted[ticketRef{epicIdx: epicIdx, ticketIdx: ticketIdx}] = true
			}
		}
	}

	if len(wanted) == 0 {
		m.search.SetMatches(nil)
		m.sidebarTree.Search().SetPassiveResults("", nil)
		m.refreshSidebarCollapse()
		m.sidebarTree.SetEntries(m.buildSidebarEntries())
		return
	}

	// deriveCollapsedSidebar (via refreshSidebarCollapse) computes the same
	// per-epic/section auto-expand this loop used to write directly into
	// m.sidebarTree's collapsed map — but transiently, off m.search.Query()
	// (already set by the caller), never touching m.explicitCollapsed.
	m.refreshSidebarCollapse()
	m.sidebarTree.SetEntries(m.buildSidebarEntries())

	matches := make([]search.Match, 0, len(wanted))
	for i, e := range m.sidebarTree.Entries() {
		// Archived nodes index m.archivedEpics, not m.epics, so their
		// epicIdx would collide with an unrelated wanted ref.
		if (e.Value.kind != nodeTicket && e.Value.kind != nodeEpic) || e.Value.archived {
			continue
		}
		if wanted[ticketRef{epicIdx: e.Value.epicIdx, ticketIdx: e.Value.ticketIdx}] {
			matches = append(matches, search.Match{DataIndex: i})
		}
	}
	m.search.SetMatches(matches)
	m.sidebarTree.Search().SetPassiveResults(q, matches)
}

// jumpToCurrentMatch moves the selection to the search cursor's current
// match, mirroring ui/log's jumpToCurrentMatch. No header-skip needed —
// matches are never nodeSection entries.
func (m *Model) jumpToCurrentMatch() {
	match, ok := m.search.Match(m.search.Cursor())
	if !ok {
		return
	}
	entries := m.sidebarTree.Entries()
	if match.DataIndex >= 0 && match.DataIndex < len(entries) {
		m.sidebarTree.SetSelectedIndex(match.DataIndex)
	}
}

// searchMatch reports whether the sidebar entry at idx is a search match,
// and whether it's the match currently under the search cursor (n/N target).
func (m Model) searchMatch(idx int) (matched, current bool) {
	return m.sidebarTree.SearchMatch(idx)
}
