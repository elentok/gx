// Package prstate derives the pull-request flow's epic states from the ticket
// store alone. Live PR data (draft, CI result) is not read here. It is a leaf of
// server/epics so that package stays inside its size budget.
package prstate

import (
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// DeliveryPR is the project's delivery value for pull requests.
const DeliveryPR = "pr"

// IsBlockKind reports whether a stored land_block_kind is a PR-flow pause
// rather than a merge pause.
func IsBlockKind(kind string) bool {
	return kind == string(schema.PauseCIFailing) || kind == string(schema.PausePRTextFailed)
}

// Blocked reports whether e is complete and unmerged with a PR-flow pause
// stored on it.
func Blocked(e tickets.Epic) bool {
	return !e.IsMap && e.MergedAt.IsZero() && IsBlockKind(e.MergeBlockKind) && remaining(e) == ""
}

// Of is e's PR-flow state, or "" when it has none and the merge rules decide.
// The caller must not ask for a one-off, a paused epic's PR block or a merge
// block; Of returns "" for a blocked epic. A ci-fix or pr-text ticket that is
// paused leaves the epic to the paused rules.
func Of(e tickets.Epic, delivery string) wire.EpicState {
	if e.IsMap || !e.MergedAt.IsZero() {
		return ""
	}
	switch remaining(e) {
	case "other":
		return ""
	case string(schema.TypePRText):
		return wire.EpicStateWritingPRText
	case string(schema.TypeCIFix):
		return wire.EpicStateFixingCI
	}
	switch {
	case Blocked(e):
		return ""
	case e.PRNumber != 0:
		return wire.EpicStatePROpen
	case delivery == DeliveryPR:
		return wire.EpicStateAwaitingPR
	}
	return ""
}

// remaining names what keeps e unfinished: "" when every ticket is done or
// cancelled, the ticket type when only open or claimed pr-text or ci-fix
// tickets of that one type remain, else "other".
func remaining(e tickets.Epic) string {
	only := ""
	for _, t := range e.Tickets {
		switch e.RenderedStatus(t) {
		case tickets.StatusDone, tickets.StatusCancelled:
		case tickets.StatusOpen, tickets.StatusClaimed:
			if e.IsPaused(t) || (t.Type != string(schema.TypePRText) && t.Type != string(schema.TypeCIFix)) || (only != "" && only != t.Type) {
				return "other"
			}
			only = t.Type
		default:
			return "other"
		}
	}
	return only
}

// CIFixAttempt is the number of ci-fix tickets created since the last reset.
func CIFixAttempt(e tickets.Epic, since time.Time) int {
	n := 0
	for _, t := range e.Tickets {
		if t.Type == string(schema.TypeCIFix) && !t.CreatedAt.Before(since) {
			n++
		}
	}
	return n
}
