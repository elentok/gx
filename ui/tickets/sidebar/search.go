package sidebar

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/tickets/rowfmt"
)

// ticketRef identifies a ticket by its epic and position within that epic's
// Tickets slice, independent of Tree's entries/collapse state.
type ticketRef struct {
	epicIdx   int
	ticketIdx int
}

// UpdateSearch feeds msg to the search and keeps matches and selection in
// step with it. handled is false when the search ignored the key.
func (s *Model) UpdateSearch(env Env, msg tea.Msg) (cmd tea.Cmd, handled bool) {
	next, cmd, result := s.Search.Update(msg)
	if !result.Handled {
		return nil, false
	}
	s.Search = next
	if result.QueryChanged {
		s.RecomputeSearchMatches(env)
	}
	if result.QueryChanged || result.CursorChanged {
		s.JumpToCurrentMatch()
	}
	return cmd, true
}

// RecomputeSearchMatches matches against the underlying epic/ticket data
// directly (s.Epics), not s.Tree.Entries(), so a match inside a
// collapsed epic or a collapsed Closed section (see DeriveCollapsed)
// is still found: TicketMatchesQuery for ticket rows, EpicMatchesQuery for
// epic rows. Any epic containing a match is auto-expanded first, along with
// its containing section (via Tree's collapsed IDs), so the match's
// eventual DataIndex (looked up post-expansion, post-rebuild) lands on an
// entry that's actually rendered. Section header rows never match. A query
// matching nothing leaves collapse state untouched.
func (s *Model) RecomputeSearchMatches(env Env) {
	q := strings.ToLower(strings.TrimSpace(s.Search.Query()))
	if q == "" {
		s.Search.SetMatches(nil)
		// Tree's own internal search.Model backs SearchMatch's per-row lookup
		// (the dim/highlight in view.go): its HasQuery() gates SearchMatch, so
		// the query has to land there too, not just on the matches —
		// SetPassiveResults sets both in one call.
		s.Tree.Search().SetPassiveResults("", nil)
		// Re-derive collapse state now (rather than waiting for the next
		// unrelated rebuild) so ending a search drops its transient
		// auto-expand override immediately, per ticket 02.
		s.Rebuild(env)
		return
	}

	// An epic row is keyed as ticketRef{epicIdx, -1}, matching its Node's
	// TicketIdx.
	wanted := map[ticketRef]bool{}
	for epicIdx, epic := range s.Epics {
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
		s.Search.SetMatches(nil)
		s.Tree.Search().SetPassiveResults("", nil)
		s.Rebuild(env)
		return
	}

	// Rebuild computes the per-epic/section auto-expand transiently, off
	// s.Search.Query() (already set by the caller), never touching
	// ExplicitCollapsed.
	s.Rebuild(env)

	matches := make([]search.Match, 0, len(wanted))
	for i, e := range s.Tree.Entries() {
		// Archived nodes index ArchivedEpics, not Epics, so their
		// EpicIdx would collide with an unrelated wanted ref.
		if (e.Value.Kind != NodeTicket && e.Value.Kind != NodeEpic) || e.Value.Archived {
			continue
		}
		if wanted[ticketRef{epicIdx: e.Value.EpicIdx, ticketIdx: e.Value.TicketIdx}] {
			matches = append(matches, search.Match{DataIndex: i})
		}
	}
	s.Search.SetMatches(matches)
	s.Tree.Search().SetPassiveResults(q, matches)
}

// JumpToCurrentMatch moves the selection to the search cursor's current
// match, mirroring ui/log's jumpToCurrentMatch. No header-skip needed —
// matches are never section entries.
func (s *Model) JumpToCurrentMatch() {
	match, ok := s.Search.Match(s.Search.Cursor())
	if !ok {
		return
	}
	entries := s.Tree.Entries()
	if match.DataIndex >= 0 && match.DataIndex < len(entries) {
		s.Tree.SetSelectedIndex(match.DataIndex)
	}
}
