// Package schedule is the ticket-scheduling core for `gx ralph-loop`:
// computing which tickets in an epic are ready to hand to an agent, which
// run scope they belong to, and how an epic's tickets tally by status.
package schedule

import (
	"cmp"
	"sort"
	"strings"

	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/tickets"
)

// defaultMaxParallel is how many iterations run concurrently when no
// MaxParallel is configured.
const defaultMaxParallel = 2

// TicketVerdict is the scheduler's single per-ticket eligibility decision:
// why t was or wasn't claimed this pass, with the reason a scan line reports.
// The first matching rule wins, so the order is the precedence.
func TicketVerdict(epic tickets.Epic, scope RunScope, t tickets.Ticket, claimed, inFrontier bool) runlog.ScanDecision {
	status := epic.RenderedStatus(t)
	d := runlog.ScanDecision{Ticket: t.Identifier, Status: status.Word()}
	switch {
	case claimed:
		d.Decision = runlog.DecisionClaimed
	case !scope.Contains(t, epic):
		d.Decision = runlog.DecisionOutOfScope
	case status.Terminal():
		d.Decision = runlog.DecisionDone
	case status == tickets.StatusError:
		d.Decision = runlog.DecisionError
		d.Reason = cmp.Or(t.ReadErr, t.GraphErr, t.BlockedByErr, "unrecognized status "+t.Status)
	case epic.IsPaused(t):
		d.Decision = runlog.DecisionStalled
	case inFrontier:
		d.Decision = runlog.DecisionFrontier
	case status == tickets.StatusBlocked:
		d.Decision = runlog.DecisionBlocked
		d.Reason = strings.Join(epic.UnresolvedBlockers(t), ", ")
	default:
		d.Decision = runlog.DecisionUnclaimed
	}
	return d
}

// AllDone reports whether every ticket in e is done — the run's one exit
// condition. A stalled ticket (see tickets.Epic.IsPaused) is deliberately not an
// exit condition: the run pauses on it instead, so the agent's question is
// still answerable in its own pane.
func AllDone(e tickets.Epic) bool {
	if len(e.Tickets) == 0 {
		return false
	}
	for _, t := range e.Tickets {
		if !e.RenderedStatus(t).Terminal() {
			return false
		}
	}
	return true
}

// LoadNamedEpic loads scratchDir and returns the epic named name, or nil if
// no such epic exists.
func LoadNamedEpic(scratchDir, name string) (*tickets.Epic, error) {
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		return nil, err
	}
	for i := range epics {
		if epics[i].Name == name {
			return &epics[i], nil
		}
	}
	return nil, nil
}

// Frontier returns e's open, unblocked, unclaimed tickets, lowest ticket
// number first. It's a thin wrapper over Epic.RenderedStatus: a ticket only
// renders as StatusOpen once its own Status: is unclaimed/missing and every
// Blocked by: number is done, so filtering on that single state gives the
// frontier for free. A map epic has no frontier: its tickets are decisions a
// person resolves by hand, so no scheduler ever claims one.
func Frontier(e tickets.Epic) []tickets.Ticket {
	if e.IsMap {
		return nil
	}
	var frontier []tickets.Ticket
	for _, t := range e.Tickets {
		if e.RenderedStatus(t) == tickets.StatusOpen {
			frontier = append(frontier, t)
		}
	}
	sort.Slice(frontier, func(i, j int) bool { return frontier[i].Number < frontier[j].Number })
	return frontier
}
