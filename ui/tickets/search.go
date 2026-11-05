package tickets

import (
	"strings"

	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/tickets/rowfmt"
	"github.com/elentok/gx/ui/tickets/sidebar"
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
		m.sidebar.Tree.Search().SetPassiveResults("", nil)
		// Re-derive collapse state now (rather than waiting for the next
		// unrelated rebuild) so ending a search drops its transient
		// auto-expand override immediately, per ticket 02.
		m.sidebar.Rebuild(sidebarEnvFor(*m))
		return
	}

	// An epic row is keyed as ticketRef{epicIdx, -1}, matching its
	// sidebarNode's ticketIdx.
	wanted := map[ticketRef]bool{}
	for epicIdx, epic := range m.sidebar.Epics {
		if rowfmt.EpicMatchesQuery(epic, q) {
			wanted[ticketRef{epicIdx: epicIdx, ticketIdx: -1}] = true
		}
		for ticketIdx, t := range epic.Tickets {
			if rowfmt.TicketMatchesQuery(epic, t, q) {
				wanted[ticketRef{epicIdx: epicIdx, ticketIdx: ticketIdx}] = true
			}
		}
	}

	if len(wanted) == 0 {
		m.search.SetMatches(nil)
		m.sidebar.Tree.Search().SetPassiveResults("", nil)
		m.sidebar.Rebuild(sidebarEnvFor(*m))
		return
	}

	// deriveCollapsedSidebar (via rebuild) computes the same per-epic/section
	// auto-expand this loop used to write directly into the tree's collapsed
	// map — but transiently, off m.search.Query() (already set by the
	// caller), never touching explicitCollapsed.
	m.sidebar.Rebuild(sidebarEnvFor(*m))

	matches := make([]search.Match, 0, len(wanted))
	for i, e := range m.sidebar.Tree.Entries() {
		// Archived nodes index m.archivedEpics, not m.epics, so their
		// epicIdx would collide with an unrelated wanted ref.
		if (e.Value.Kind != sidebar.NodeTicket && e.Value.Kind != sidebar.NodeEpic) || e.Value.Archived {
			continue
		}
		if wanted[ticketRef{epicIdx: e.Value.EpicIdx, ticketIdx: e.Value.TicketIdx}] {
			matches = append(matches, search.Match{DataIndex: i})
		}
	}
	m.search.SetMatches(matches)
	m.sidebar.Tree.Search().SetPassiveResults(q, matches)
}

// jumpToCurrentMatch moves the selection to the search cursor's current
// match, mirroring ui/log's jumpToCurrentMatch. No header-skip needed —
// matches are never nodeSection entries.
func (m *Model) jumpToCurrentMatch() {
	match, ok := m.search.Match(m.search.Cursor())
	if !ok {
		return
	}
	entries := m.sidebar.Tree.Entries()
	if match.DataIndex >= 0 && match.DataIndex < len(entries) {
		m.sidebar.Tree.SetSelectedIndex(match.DataIndex)
	}
}
