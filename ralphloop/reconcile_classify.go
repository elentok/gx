package ralphloop

import (
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
)

// doneMismatchClass is how a done ticket's recorded landed commit compares
// against the feature branch's current tip, plus whether its iteration state
// was left behind — see classifyDoneTicket.
type doneMismatchClass int

const (
	doneOK doneMismatchClass = iota
	doneStaleCleanup
	doneRecoverable
	doneUnrecoverable
)

// classifyDoneTicket folds a ticket's TicketVerification (see VerifyEpic for
// the three-rung landing ladder) into one of four classifications:
//
//   - doneOK: the commit landed and nothing was left behind.
//   - doneStaleCleanup: the commit landed, but a tab/worktree/branch is
//     still around — the crash landed between marking done and the cleanup
//     step right after it.
//   - doneRecoverable: the commit is missing, but the iteration branch still
//     holds it.
//   - doneUnrecoverable: the commit is missing and no iteration branch is
//     left to recover it from.
//
// A verification whose checks couldn't run leaves the ticket untouched
// (doneOK) unless a check failed outright, which surfaces as an error.
func classifyDoneTicket(d Deps, paths reconcilePaths, featureBranch string, t tickets.Ticket, events []Event, live map[string]bool, landed map[string]bool) (doneMismatchClass, error) {
	return foldVerification(verifyTicket(d.verifyDeps(), paths, featureBranch, t, events, live, landed, nil))
}

func foldVerification(v TicketVerification) (doneMismatchClass, error) {
	if v.err != nil {
		return doneOK, v.err
	}
	switch v.Landing {
	case LandingLanded:
		if isTrue(v.Leftovers.Tab) || isTrue(v.Leftovers.Worktree) || isTrue(v.Leftovers.Branch) {
			return doneStaleCleanup, nil
		}
		return doneOK, nil
	case LandingRecoverable:
		return doneRecoverable, nil
	case LandingUnrecoverable:
		return doneUnrecoverable, nil
	default:
		return doneOK, nil
	}
}

func isTrue(b *bool) bool { return b != nil && *b }

// latestCherryPickedSHA returns the SHA recorded on the most recent
// cherry-picked event logged for identifier (a ticket's Identifier, not
// Number, so lettered split siblings sharing a Number aren't
// cross-attributed), or "" if none was ever logged.
func latestCherryPickedSHA(evs []Event, identifier string) string {
	sha := ""
	for _, ev := range evs {
		if ev.Type == string(events.CherryPicked) && ev.Ticket == identifier && ev.SHA != "" {
			sha = ev.SHA
		}
	}
	return sha
}
