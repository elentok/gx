package merge

import (
	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/tickets"
)

// EpicTrailerLines counts the lines added and removed by the commits reachable
// from ref whose Ralph-Loop-Ticket trailer names epic. It stamps an epic that
// merged outside the server, where no trunk before/after pair exists.
func EpicTrailerLines(dir, ref, epic string) (added, removed int, err error) {
	return history.TrailerLines(dir, ref, TicketTrailerKey, func(value string) bool {
		e, _, ok := tickets.SplitTrailerValue(value)
		return ok && e == epic
	})
}

// MergedTickets reports which tickets have a commit reachable from
// featureBranch stamped with their Ralph-Loop-Ticket trailer (see
// TicketTrailerValue), keyed by ticket identifier — a single history.TrailerMap
// call replacing what would otherwise be one TrailerCommitExists shell-out
// per ticket. It is VerifyEpic's trailer rung. A returned error means the
// check itself couldn't run (e.g. dir isn't a git repo, or featureBranch
// doesn't exist) — callers must keep that "unknown" outcome distinct from a
// ticket simply being absent from a successfully-computed map ("not merged").
func MergedTickets(dir, featureBranch string) (map[string]bool, error) {
	trailers, err := history.TrailerMap(dir, featureBranch, TicketTrailerKey)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]bool, len(trailers))
	for value := range trailers {
		epic, identifier, ok := tickets.SplitTrailerValue(value)
		if !ok || epic != featureBranch {
			// Unscoped or cross-epic trailer value (see
			// TicketTrailerValue's epic-scoping rationale) — never counts
			// as this epic's ticket merging.
			continue
		}
		merged[identifier] = true
	}
	return merged, nil
}
