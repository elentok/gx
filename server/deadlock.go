package server

import (
	"slices"
	"strings"
	"sync"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/tickets"
)

// deadlocks is the roots now in deadlock, so a root logs deadlocked once per
// entry rather than on every claim pass.
type deadlocks struct {
	mu    sync.Mutex
	roots map[rootRef]bool
}

// enter reports whether root was not already in deadlock.
func (d *deadlocks) enter(root rootRef) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.roots[root] {
		return false
	}
	if d.roots == nil {
		d.roots = map[rootRef]bool{}
	}
	d.roots[root] = true
	return true
}

func (d *deadlocks) leave(root rootRef) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.roots, root)
}

// isParked is tickets.Epic.IsParked, the rule shared with ralph-loop and the
// Queue tab, except that map epics hold no work, so none of their tickets are.
func isParked(e tickets.Epic, t tickets.Ticket) bool {
	return !e.IsMap && e.IsParked(t)
}

// epicState is how far e can move.
type epicState int

const (
	// epicDone: no work remains (also every map epic, which holds none).
	epicDone epicState = iota
	// epicRunnable: a ticket is open or claimed, so the run can proceed.
	epicRunnable
	// epicParked: nothing runnable, and a person can clear what remains.
	epicParked
	// epicCycle: nothing runnable and nothing a person can clear, only blocked
	// tickets.
	epicCycle
)

// epicStateOf is the server's one rule for an epic's state; the deadlock check and
// the epic summary both derive from it. A status it does not recognize counts as
// parked, never as a cycle: a person has to look at it.
func epicStateOf(e tickets.Epic) epicState {
	if e.IsMap {
		return epicDone
	}
	state := epicDone
	for _, t := range e.Tickets {
		if isParked(e, t) {
			state = max(state, epicParked)
			continue
		}
		switch e.RenderedStatus(t) {
		case tickets.StatusOpen, tickets.StatusClaimed:
			return epicRunnable
		case tickets.StatusDone, tickets.StatusCancelled:
		case tickets.StatusBlocked, tickets.StatusWaitingForChildren:
			if state == epicDone {
				state = epicCycle
			}
		default:
			state = epicParked
		}
	}
	return state
}

// deadlockKind says how e is deadlocked: nothing schedulable and nothing
// running while work remains. Parked tickets make it all-parked, since a person
// can clear them; none makes it a blocked cycle.
func deadlockKind(e tickets.Epic) (events.Kind, bool) {
	switch epicStateOf(e) {
	case epicParked:
		return events.AllParked, true
	case epicCycle:
		return events.BlockedCycle, true
	}
	return "", false
}

// deadlockReason names what holds e: the parked tickets for all-parked, and for
// a blocked cycle each waiting ticket with what it waits on.
func deadlockReason(root rootRef, e tickets.Epic, kind events.Kind) string {
	if kind == events.AllParked {
		return parkedReason(root, e)
	}
	return cycleReason(root, e)
}

func ticketAddr(root rootRef, t tickets.Ticket) string { return root.String() + "/" + t.Identifier }

func parkedReason(root rootRef, e tickets.Epic) string {
	var parts []string
	for _, t := range e.Tickets {
		if isParked(e, t) {
			parts = append(parts, ticketAddr(root, t))
		}
	}
	return "every remaining ticket is parked: " + strings.Join(parts, ", ")
}

func cycleReason(root rootRef, e tickets.Epic) string {
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
		parts = append(parts, ticketAddr(root, t)+" waits on "+strings.Join(on, ", "))
	}
	return strings.Join(parts, "; ")
}

// checkDeadlock logs deadlocked when root's epic enters deadlock and starts its
// recovery; it forgets the deadlock once the epic can move again.
func (s *Server) checkDeadlock(root rootRef, projectDir string, e tickets.Epic) {
	kind, stuck := deadlockKind(e)
	if !stuck || s.registry.countRoot(root.String()) > 0 {
		s.deadlocks.leave(root)
		return
	}
	if !s.deadlocks.enter(root) {
		return
	}
	reason := deadlockReason(root, e, kind)
	ev := ralphloop.Event{Type: string(events.Deadlocked), Kind: string(kind), Reason: reason}
	if err := ralphloop.AppendEvent(projectDir, root.Epic, ev); err != nil {
		s.log.Warn("cannot record deadlock", "root", root, "err", err)
	}
	s.recoverAsync(recovery.Failure{Address: root.String(), Type: events.Deadlocked, Kind: kind, Reason: reason})
}
