package ralphloop

import (
	"strings"

	"github.com/elentok/gx/git"
)

// LandedTickets reports which tickets have a commit reachable from
// featureBranch stamped with their Ralph-Loop-Ticket trailer (see
// ticketTrailerValue), keyed by ticket identifier — a single git.TrailerMap
// call replacing what would otherwise be one TrailerCommitExists shell-out
// per ticket. Exported so ui/tickets (a read-only display, with no need for
// classifyDoneTicket's full IsAncestor/PatchesApplied/worktree chain) can
// call it directly. A returned error means the check itself couldn't run
// (e.g. dir isn't a git repo, or featureBranch doesn't exist) — callers must
// keep that "unknown" outcome distinct from a ticket simply being absent
// from a successfully-computed map ("not landed").
func LandedTickets(dir, featureBranch string) (map[string]bool, error) {
	trailers, err := git.TrailerMap(dir, featureBranch, ticketTrailerKey)
	if err != nil {
		return nil, err
	}
	prefix := featureBranch + "/"
	landed := make(map[string]bool, len(trailers))
	for value := range trailers {
		identifier, ok := strings.CutPrefix(value, prefix)
		if !ok {
			// Unscoped or cross-epic trailer value (see
			// ticketTrailerValue's epic-scoping rationale) — never counts
			// as this epic's ticket landing.
			continue
		}
		landed[identifier] = true
	}
	return landed, nil
}
