package ralphloop

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/tickets"
)

// pausedIdentifierCap bounds how many paused identifiers a counts line lists
// inline before falling back to an overflow marker — enough to be useful
// without a pause-heavy epic turning one chat message into a wall of ids.
const pausedIdentifierCap = 5

// EpicCounts tallies an epic's tickets by rendered status, the input to
// RenderCountsLine. Done and Total are the two fields that describe the
// whole epic regardless of bucket; the rest name what's keeping the
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
// lands in Blocked, not Done, matching DoneCount's existing rule that an
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
		case tickets.StatusNeedsAnswer, tickets.StatusNeedsRepair:
			counts.PausedIdentifiers = append(counts.PausedIdentifiers, ticket.Identifier)
		case tickets.StatusBlocked, tickets.StatusWaitingForChildren:
			counts.Blocked++
		case tickets.StatusOpen:
			counts.Ready++
		}
	}
	return counts
}

// RenderCountsLine renders c in the fixed bucket order done, in progress,
// paused, blocked, ready, total. Every bucket but done/total is suppressed
// at zero, so a line only names what's materially true rather than
// repeating "0 blocked" on every message. This is a pure function of c —
// the formatting decisions live here, not in any sink — which is what makes
// it table-testable independent of chat transport or epic state.
func RenderCountsLine(c EpicCounts) string {
	clauses := []string{fmt.Sprintf("%d done", c.Done)}
	if c.InProgress > 0 {
		clauses = append(clauses, fmt.Sprintf("%d in progress", c.InProgress))
	}
	if len(c.PausedIdentifiers) > 0 {
		clauses = append(clauses, fmt.Sprintf("%d paused: %s", len(c.PausedIdentifiers), joinPausedIdentifiers(c.PausedIdentifiers)))
	}
	if c.Blocked > 0 {
		clauses = append(clauses, fmt.Sprintf("%d blocked", c.Blocked))
	}
	if c.Ready > 0 {
		clauses = append(clauses, fmt.Sprintf("%d ready", c.Ready))
	}
	clauses = append(clauses, fmt.Sprintf("%d total", c.Total))
	return strings.Join(clauses, " · ")
}

// loadEpicCounts loads epicName's current on-disk state from scratchDir and
// tallies it into EpicCounts, epic-truth-fresh at the moment a chat message
// sends rather than reused from any earlier read — the epic-level messages
// this feeds (epicStartedText/epicCompleteText/ticketNeedsHumanText) fire
// rarely enough that a dedicated load here doesn't repeat the per-poll cost
// ticket 26 avoided for done/total. Returns the zero value if the epic can't
// be loaded, so a transient read hiccup drops the counts clause instead of
// the whole notification.
func loadEpicCounts(scratchDir, epicName string) EpicCounts {
	epic, err := loadNamedEpic(scratchDir, epicName)
	if err != nil || epic == nil {
		return EpicCounts{}
	}
	return RunScope{wholeEpic: true}.Counts(*epic)
}

// loadEpicTotalCost sums ActualCost across every ticket in epicName, the
// epic-wide counterpart to a single landed ticket's IterationStats.Cost.
// Same load-fresh, zero-on-failure posture as loadEpicCounts.
func loadEpicTotalCost(scratchDir, epicName string) float64 {
	epic, err := loadNamedEpic(scratchDir, epicName)
	if err != nil || epic == nil {
		return 0
	}
	var total float64
	for _, t := range epic.Tickets {
		total += t.ActualCost
	}
	return total
}

// joinPausedIdentifiers lists identifiers inline, capped at
// pausedIdentifierCap: beyond that, the rest collapse into a single "+N
// more" marker instead of growing the line without bound.
func joinPausedIdentifiers(identifiers []string) string {
	if len(identifiers) <= pausedIdentifierCap {
		return strings.Join(identifiers, ", ")
	}
	shown := strings.Join(identifiers[:pausedIdentifierCap], ", ")
	return fmt.Sprintf("%s, +%d more", shown, len(identifiers)-pausedIdentifierCap)
}

// CountsOf tallies a whole epic the way the chat messages show it.
func CountsOf(e tickets.Epic) EpicCounts { return RunScope{wholeEpic: true}.Counts(e) }

// EpicTotals is the epic's summed landing time (seconds) and cost, for the
// "epic complete" message of a run that did not time it itself.
func EpicTotals(e tickets.Epic) (elapsedSeconds int, cost float64) {
	for _, t := range e.Tickets {
		elapsedSeconds += t.ElapsedTime
		cost += t.ActualCost
	}
	return elapsedSeconds, cost
}
