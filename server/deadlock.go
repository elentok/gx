package server

import (
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

// isParked is the server's one rule for a Parked ticket: one a person, not the
// run, has to clear. Map epics hold no work, so none of their tickets are.
func isParked(e tickets.Epic, t tickets.Ticket) bool {
	if e.IsMap {
		return false
	}
	switch e.RenderedStatus(t) {
	case tickets.StatusNeedsAnswer, tickets.StatusNeedsRepair, tickets.StatusDraft, tickets.StatusError:
		return true
	}
	return false
}

// deadlockKind says how e is deadlocked: nothing schedulable and nothing
// running while work remains. Parked tickets make it all-parked, since a person
// can clear them; none makes it a blocked cycle.
func deadlockKind(e tickets.Epic) (events.Kind, bool) {
	if e.IsMap {
		return "", false
	}
	remaining, parked := false, false
	for _, t := range e.Tickets {
		if isParked(e, t) {
			parked = true
			remaining = true
			continue
		}
		switch e.RenderedStatus(t) {
		case tickets.StatusOpen, tickets.StatusClaimed:
			return "", false
		case tickets.StatusDone, tickets.StatusCancelled:
			continue
		}
		parked = parked || e.IsParked(t)
		remaining = true
	}
	switch {
	case !remaining:
		return "", false
	case parked:
		return events.AllParked, true
	}
	return events.BlockedCycle, true
}

// deadlockReason names what holds e: the parked tickets for all-parked, and for
// a blocked cycle each waiting ticket with what it waits on.
func deadlockReason(root rootRef, e tickets.Epic, kind events.Kind) string {
	addr := func(t tickets.Ticket) string { return root.String() + "/" + t.Identifier }
	var parts []string
	for _, t := range e.Tickets {
		if kind == events.AllParked {
			if isParked(e, t) {
				parts = append(parts, addr(t))
			}
			continue
		}
		if e.RenderedStatus(t) == tickets.StatusBlocked {
			waits := e.UnresolvedBlockers(t)
			if !e.ParentDone(t) {
				waits = append(waits, *t.Parent)
			}
			var on []string
			for _, b := range waits {
				if !strings.Contains(b, ":") {
					b = root.String() + "/" + b
				}
				on = append(on, b)
			}
			parts = append(parts, addr(t)+" waits on "+strings.Join(on, ", "))
		}
	}
	if kind == events.AllParked {
		return "every remaining ticket is parked: " + strings.Join(parts, ", ")
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
