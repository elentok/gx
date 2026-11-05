package schedule

import "github.com/elentok/gx/tickets"

// EpicCounts tallies an epic's tickets by rendered status, the input to
// ralphloop.RenderCountsLine. Done and Total are the two fields that describe
// the whole epic regardless of bucket; the rest name what's keeping the
// remainder from being done. PausedIdentifiers drives the inline "paused:
// 07, 12" clause — its length is the paused count, so there is exactly one
// source of truth for how many are paused.
type EpicCounts struct {
	Done              int
	InProgress        int
	PausedIdentifiers []string
	Blocked           int
	Ready             int
	Total             int
	// Recovered is how many pauses recovery fixed without a person; only the
	// server fills it, for the epic-complete summary.
	Recovered int
}

// Counts tallies epic's tickets into EpicCounts, restricted to s's
// membership exactly like DoneCount/TotalCount. A done parent still waiting
// on its fork subtree (Epic.RenderedStatus's StatusWaitingForChildren)
// merges in Blocked, not Done, matching DoneCount's existing rule that an
// outstanding fork subtree is outstanding work.
func (s RunScope) Counts(epic tickets.Epic) EpicCounts {
	counts := EpicCounts{Total: s.TotalCount(epic)}
	for _, ticket := range epic.Tickets {
		if !s.wholeEpic && !s.Contains(ticket, epic) {
			continue
		}
		switch epic.RenderedStatus(ticket) {
		case tickets.StatusDone, tickets.StatusCancelled:
			counts.Done++
		case tickets.StatusClaimed:
			counts.InProgress++
		case tickets.StatusPausedAnswer, tickets.StatusPausedRepair:
			counts.PausedIdentifiers = append(counts.PausedIdentifiers, ticket.Identifier)
		case tickets.StatusBlocked, tickets.StatusWaitingForChildren:
			counts.Blocked++
		case tickets.StatusOpen:
			counts.Ready++
		}
	}
	return counts
}

// CountsOf tallies a whole epic the way the chat messages show it.
func CountsOf(e tickets.Epic) EpicCounts { return WholeEpicScope().Counts(e) }

// EpicTotals is the epic's summed merging time (seconds) and cost, for the
// "epic complete" message of a run that did not time it itself.
func EpicTotals(e tickets.Epic) (elapsedSeconds int, cost float64) {
	for _, t := range e.Tickets {
		elapsedSeconds += t.ElapsedTime
		cost += t.ActualCost
	}
	return elapsedSeconds, cost
}
