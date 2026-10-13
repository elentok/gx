package server

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// maxMergeTickets is how many merge tickets one run gets before the root parks.
const maxMergeTickets = 2

// mergeTicketsThisRun counts e's merge tickets from the latest manual one
// onward, that one included; all of them when none is manual. The store is the
// count's only record, so a restart keeps it.
func mergeTicketsThisRun(e tickets.Epic) int {
	merges := make([]tickets.Ticket, 0)
	for _, t := range e.Tickets {
		if t.Type == string(schema.TypeMerge) {
			merges = append(merges, t)
		}
	}
	slices.SortFunc(merges, func(a, b tickets.Ticket) int {
		return cmp.Or(cmp.Compare(a.Number, b.Number), cmp.Compare(a.Identifier, b.Identifier))
	})
	for i := len(merges) - 1; i >= 0; i-- {
		if merges[i].Manual {
			return len(merges) - i
		}
	}
	return len(merges)
}

// hasManualMergeTicket reports whether e holds a merge ticket the manual
// trigger forked. Unmerged, such an epic lands whatever auto-ff-merge says:
// the rule reads the store, so a retry ticket finishing last still lands.
func hasManualMergeTicket(e tickets.Epic) bool {
	return slices.ContainsFunc(e.Tickets, func(t tickets.Ticket) bool {
		return t.Type == string(schema.TypeMerge) && t.Manual
	})
}

// forkMergeTicket forks a merge ticket into e for its non-ff onto target and
// returns its id. At the cap, or when the write fails, it forks none and the
// caller parks the root. A manual ticket, the manual trigger's, skips the cap.
func (s *Server) forkMergeTicket(root rootRef, e tickets.Epic, target string, manual bool) string {
	if !manual && mergeTicketsThisRun(e) >= maxMergeTickets {
		return ""
	}
	_, id, err := writeNewTicket(e.Path, "", "merge", func(id string) (schema.Ticket, string) {
		tk := schema.Ticket{ID: schema.TicketID(id), Status: schema.StatusOpen, Type: schema.TypeMerge, Manual: manual}
		body := fmt.Sprintf("\n# %s — Merge %s onto %s\n\n## What to build\n\nRebase %s onto %s so it can fast-forward. The server lands it once this ticket is done.\n",
			id, root.Epic, target, root.Epic, target)
		return tk, body
	})
	if err != nil {
		s.log.Warn("fork merge ticket", "root", root, "err", err)
		return ""
	}
	s.log.Info("merge ticket forked", "root", root, "ticket", id, "manual", manual)
	s.events.publish(EventQueueChanged, root.String())
	return id
}
