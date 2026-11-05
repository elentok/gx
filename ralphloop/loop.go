package ralphloop

import (
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/ralphloop/schedule"
	"github.com/elentok/gx/tickets"
)

// ticketAddress is the canonical "project:epic/NN" address of the ticket
// file at ticketPath (<root>/<epic>/issues/NN-<slug>.md), the only form
// iteration prompts hand to agents.
func ticketAddress(t tickets.Ticket) string {
	a, _ := tickets.AddressOfPath(t.Path, t.Identifier)
	return a.String()
}

// outcome is one iteration's or one merge's result.
type outcome struct {
	ticket tickets.Ticket
	err    error
	// built is true when this outcome is a build that finished with commits
	// ready to merge (ahead > 0): iteration_status: finished is already
	// written.
	built bool
	// pausedOnChild is true when this outcome is a merge outcome whose
	// cherry-pick hit errConflictResolutionUnresolved: the ticket was paused
	// (repair) on a conflict-resolution child, not merged — it stays claimed on
	// disk. err is nil on this path (pausing already handled the failure).
	pausedOnChild bool
	// childReason names the conflict-resolution child and why it failed, on a
	// pausedOnChild outcome; the server pauses the parent with it.
	childReason string
	// mergeDeferred is set by mergeOne when the merge lock was held or a human's
	// conflict marker is pending.
	mergeDeferred bool
}

// scanDecisions builds one claimNext pass's scheduler-scan log line: epic's
// tickets, in file order, each labeled with why the scheduler did or didn't
// claim it this pass. claimed is the zero-value Ticket when this pass found
// nothing to claim.
func scanDecisions(epic tickets.Epic, scope schedule.RunScope, frontier []tickets.Ticket, claimed tickets.Ticket) []runlog.ScanDecision {
	inFrontier := make(map[string]bool, len(frontier))
	for _, t := range frontier {
		inFrontier[t.Path] = true
	}

	decisions := make([]runlog.ScanDecision, 0, len(epic.Tickets))
	for _, t := range epic.Tickets {
		isClaimed := claimed.Path != "" && t.Path == claimed.Path
		decisions = append(decisions, schedule.TicketVerdict(epic, scope, t, isClaimed, inFrontier[t.Path]))
	}
	return decisions
}
