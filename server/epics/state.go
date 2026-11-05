package epics

import (
	"slices"
	"strings"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/tickets"
)

// IsPaused is tickets.Epic.IsPaused, the rule shared with ralph-loop and the
// Queue tab, except that map epics hold no work, so none of their tickets are.
func IsPaused(e tickets.Epic, t tickets.Ticket) bool {
	return !e.IsMap && e.IsPaused(t)
}

// State is how far an epic can move.
type State int

const (
	// StateDone: no work remains (also every map epic, which holds none).
	StateDone State = iota
	// StateRunnable: a ticket is open or claimed, so the run can proceed.
	StateRunnable
	// StatePaused: nothing runnable, and a person can clear what remains.
	StatePaused
	// StateCycle: nothing runnable and nothing a person can clear, only blocked
	// tickets.
	StateCycle
)

// StateOf is the server's one rule for an epic's state; the deadlock check and
// the epic summary both derive from it. A status it does not recognize counts as
// paused, never as a cycle: a person has to look at it.
func StateOf(e tickets.Epic) State {
	if e.IsMap {
		return StateDone
	}
	state := StateDone
	for _, t := range e.Tickets {
		status := e.RenderedStatus(t)
		if status.Paused() {
			state = StatePaused
			continue
		}
		switch status {
		case tickets.StatusOpen, tickets.StatusClaimed:
			return StateRunnable
		case tickets.StatusDone, tickets.StatusCancelled:
		case tickets.StatusBlocked, tickets.StatusWaitingForChildren:
			if state == StateDone {
				state = StateCycle
			}
		default:
			state = StatePaused
		}
	}
	return state
}

// TicketAddr is t's address under root.
func TicketAddr(root queue.RootRef, t tickets.Ticket) string {
	return root.String() + "/" + t.Identifier
}

// PausedTickets are e's tickets a person can clear.
func PausedTickets(e tickets.Epic) []tickets.Ticket {
	var out []tickets.Ticket
	for _, t := range e.Tickets {
		if IsPaused(e, t) {
			out = append(out, t)
		}
	}
	return out
}

// PausedReason names the paused tickets that hold e.
func PausedReason(root queue.RootRef, e tickets.Epic) string {
	var parts []string
	for _, t := range PausedTickets(e) {
		parts = append(parts, TicketAddr(root, t))
	}
	return "every remaining ticket is paused: " + strings.Join(parts, ", ")
}

// CycleReason names each waiting ticket of a blocked cycle with what it waits on.
func CycleReason(root queue.RootRef, e tickets.Epic) string {
	ctx := tickets.AddressContext{Project: root.Project, Epic: root.Epic}
	var parts []string
	for _, t := range e.Tickets {
		if e.RenderedStatus(t) != tickets.StatusBlocked {
			continue
		}
		waits := slices.Clone(e.UnresolvedBlockers(t))
		if !e.ParentDone(t) {
			waits = append(waits, *t.Parent)
		}
		var on []string
		for _, b := range waits {
			if a, err := tickets.ParseAddress(b, ctx); err == nil {
				b = a.String()
			}
			on = append(on, b)
		}
		parts = append(parts, TicketAddr(root, t)+" waits on "+strings.Join(on, ", "))
	}
	return strings.Join(parts, "; ")
}
