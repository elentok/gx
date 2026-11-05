package schedule

import (
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/tickets"
)

// EpicQueue reads epic's queue without running it: every ticket with the
// scheduler's own verdict (TicketVerdict), whole-epic scope, a ticket already
// claimed reported as "claimed".
func EpicQueue(epic tickets.Epic) []runlog.ScanDecision {
	scope := WholeEpicScope()
	frontier := scope.Frontier(epic)
	inFrontier := make(map[string]bool, len(frontier))
	for _, t := range frontier {
		inFrontier[t.Path] = true
	}
	out := make([]runlog.ScanDecision, 0, len(epic.Tickets))
	for _, t := range epic.Tickets {
		claimed := epic.RenderedStatus(t) == tickets.StatusClaimed
		out = append(out, TicketVerdict(epic, scope, t, claimed, inFrontier[t.Path]))
	}
	return out
}
