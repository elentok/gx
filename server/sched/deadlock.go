package sched

import (
	"sync"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server/epics"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/tickets"
)

// deadlocks is the roots now in deadlock, so a root logs deadlocked once per
// entry rather than on every claim pass.
type deadlocks struct {
	mu    sync.Mutex
	roots map[queue.RootRef]bool
}

// enter reports whether root was not already in deadlock.
func (d *deadlocks) enter(root queue.RootRef) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.roots[root] {
		return false
	}
	if d.roots == nil {
		d.roots = map[queue.RootRef]bool{}
	}
	d.roots[root] = true
	return true
}

func (d *deadlocks) leave(root queue.RootRef) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.roots, root)
}

// deadlockKind says how e is deadlocked: nothing schedulable and nothing
// running while work remains. Paused tickets make it all-paused, since a person
// can clear them; none makes it a blocked cycle.
func deadlockKind(e tickets.Epic) (events.Kind, bool) {
	switch epics.StateOf(e) {
	case epics.StatePaused:
		return events.AllPaused, true
	case epics.StateCycle:
		return events.BlockedCycle, true
	}
	return "", false
}

// deadlockReason names what holds e: the paused tickets for all-paused, and for
// a blocked cycle each waiting ticket with what it waits on.
func deadlockReason(root queue.RootRef, e tickets.Epic, kind events.Kind) string {
	if kind == events.AllPaused {
		return epics.PausedReason(root, e)
	}
	return epics.CycleReason(root, e)
}

// CheckDeadlock logs deadlocked when root's epic enters deadlock and starts its
// recovery; it forgets the deadlock once the epic can move again.
func (s *Scheduler) CheckDeadlock(root queue.RootRef, projectDir string, e tickets.Epic) {
	kind, stuck := deadlockKind(e)
	if !stuck || s.Registry.CountRoot(root.String()) > 0 {
		s.deadlocks.leave(root)
		return
	}
	if !s.deadlocks.enter(root) {
		return
	}
	reason := deadlockReason(root, e, kind)
	if kind == events.AllPaused && e.AllAwaitAnswer(epics.PausedTickets(e)) {
		ev := runlog.Event{Type: string(events.WaitingForAnswer), Reason: reason}
		if err := runlog.AppendEvent(projectDir, root.Epic, ev); err != nil {
			s.Log.Warn("cannot record waiting-for-answer", "root", root, "err", err)
		}
		return
	}
	ev := runlog.Event{Type: string(events.Deadlocked), Kind: string(kind), Reason: reason}
	if err := runlog.AppendEvent(projectDir, root.Epic, ev); err != nil {
		s.Log.Warn("cannot record deadlock", "root", root, "err", err)
	}
	s.Recovery.RecoverAsync(recovery.Failure{Address: root.String(), Type: events.Deadlocked, Kind: kind, Reason: reason})
}
