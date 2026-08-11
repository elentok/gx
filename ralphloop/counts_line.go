package ralphloop

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/tickets"
)

// parkedIdentifierCap bounds how many parked identifiers a counts line lists
// inline before falling back to an overflow marker — enough to be useful
// without a park-heavy epic turning one chat message into a wall of ids.
const parkedIdentifierCap = 5

// EpicCounts tallies an epic's tickets by rendered status, the input to
// RenderCountsLine. Done and Total are the two fields that describe the
// whole epic regardless of bucket; the rest name what's keeping the
// remainder from being done. ParkedIdentifiers drives the inline "parked:
// 07, 12" clause — its length is the parked count, so there is exactly one
// source of truth for how many are parked.
type EpicCounts struct {
	Done              int
	InProgress        int
	ParkedIdentifiers []string
	Blocked           int
	Ready             int
	Total             int
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
		case tickets.StatusDone:
			counts.Done++
		case tickets.StatusClaimed:
			counts.InProgress++
		case tickets.StatusNeedsAnswer, tickets.StatusNeedsRepair:
			counts.ParkedIdentifiers = append(counts.ParkedIdentifiers, ticket.Identifier)
		case tickets.StatusBlocked, tickets.StatusWaitingForChildren:
			counts.Blocked++
		case tickets.StatusOpen:
			counts.Ready++
		}
	}
	return counts
}

// RenderCountsLine renders c in the fixed bucket order done, in progress,
// parked, blocked, ready, total. Every bucket but done/total is suppressed
// at zero, so a line only names what's materially true rather than
// repeating "0 blocked" on every message. This is a pure function of c —
// the formatting decisions live here, not in any sink — which is what makes
// it table-testable independent of chat transport or epic state.
func RenderCountsLine(c EpicCounts) string {
	clauses := []string{fmt.Sprintf("%d done", c.Done)}
	if c.InProgress > 0 {
		clauses = append(clauses, fmt.Sprintf("%d in progress", c.InProgress))
	}
	if len(c.ParkedIdentifiers) > 0 {
		clauses = append(clauses, fmt.Sprintf("%d parked: %s", len(c.ParkedIdentifiers), joinParkedIdentifiers(c.ParkedIdentifiers)))
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

// joinParkedIdentifiers lists identifiers inline, capped at
// parkedIdentifierCap: beyond that, the rest collapse into a single "+N
// more" marker instead of growing the line without bound.
func joinParkedIdentifiers(identifiers []string) string {
	if len(identifiers) <= parkedIdentifierCap {
		return strings.Join(identifiers, ", ")
	}
	shown := strings.Join(identifiers[:parkedIdentifierCap], ", ")
	return fmt.Sprintf("%s, +%d more", shown, len(identifiers)-parkedIdentifierCap)
}
