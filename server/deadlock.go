package server

import (
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

// deadlockKind says how e is deadlocked: nothing schedulable and nothing
// running while work remains. Parked tickets make it all-parked, since a person
// can clear them; none makes it a blocked cycle.
func deadlockKind(e tickets.Epic) (events.Kind, bool) {
	if e.IsMap {
		return "", false
	}
	remaining, parked := false, false
	for _, t := range e.Tickets {
		switch e.RenderedStatus(t) {
		case tickets.StatusOpen, tickets.StatusClaimed:
			return "", false
		case tickets.StatusNeedsAnswer, tickets.StatusNeedsRepair, tickets.StatusDraft:
			parked = true
		case tickets.StatusDone, tickets.StatusCancelled:
			continue
		}
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
	reason := "nothing runnable and nothing a person could clear"
	if kind == events.AllParked {
		reason = "every remaining ticket is parked"
	}
	ev := ralphloop.Event{Type: string(events.Deadlocked), Kind: string(kind), Reason: reason}
	if err := ralphloop.AppendEvent(projectDir, root.Epic, ev); err != nil {
		s.log.Warn("cannot record deadlock", "root", root, "err", err)
	}
	s.recoverAsync(recovery.Failure{Address: root.String(), Type: events.Deadlocked, Kind: kind, Reason: reason})
}
