// Package viewmodel reduces the server's snapshot and event stream into the
// state the TUI renders. It is pure: no I/O, no clock. The caller performs the
// Effect a reduction asks for and feeds the result back in.
package viewmodel

import (
	"maps"
	"slices"
	"strings"

	"github.com/elentok/gx/server"
)

// Effect is what the caller must do after a reduction. Events carry no
// payload beyond an address, so anything they can't express is a refetch.
type Effect uint8

const (
	EffectNone Effect = 0
	// EffectResnapshot: fetch /v1/snapshot and ApplySnapshot it (seq gap, or
	// an event whose outcome the stream does not describe).
	EffectResnapshot Effect = 1 << iota
	// EffectRefetchQueue: fetch the queue and SetQueue it.
	EffectRefetchQueue
)

// IterationState is a ticket's live orchestrator state, layered on its
// on-disk status.
type IterationState string

const (
	IterationClaimed IterationState = "claimed"
	IterationRunning IterationState = "running"
	IterationParked  IterationState = "parked"
	IterationFailed  IterationState = "failed"
)

// State is the render state. Treat it as immutable: Reduce and the other
// transitions return a new State and never write through the old one.
type State struct {
	Seq              uint64
	HerdrUnavailable bool
	Budget           server.BudgetStatus
	// ExtraUsage is the snapshot's extra-usage warning flag.
	ExtraUsage bool
	// Tickets is in snapshot order (by address, forks under their parent).
	Tickets []server.TicketInfo
	// Queue is the queued ticket addresses in launch order.
	Queue []string
	// Iterations holds live state by ticket address; absent means no live
	// iteration, so the row falls back to its on-disk status.
	Iterations map[string]IterationState
	// Pending is each queue entry's explain verdict, as the last snapshot
	// delivered it.
	Pending []server.PendingRow
	// CwdProject is the registered project the TUI started in; "" when the cwd
	// is not in one. It survives snapshots.
	CwdProject string
	// AllProjects shows every project instead of just CwdProject.
	AllProjects bool
}

// ScopedTickets is Tickets narrowed to the cwd project, unless the toggle says
// all or the cwd is not a registered project.
func (s State) ScopedTickets() []server.TicketInfo {
	if s.CwdProject == "" || s.AllProjects {
		return s.Tickets
	}
	prefix := s.CwdProject + ":"
	return slices.DeleteFunc(slices.Clone(s.Tickets), func(t server.TicketInfo) bool {
		return !strings.HasPrefix(t.Address, prefix)
	})
}

// ToggleAllProjects flips between the cwd project and all projects.
func (s State) ToggleAllProjects() State {
	s.AllProjects = !s.AllProjects
	return s
}

// UnregisteredHint is the hint shown when the cwd is not a registered project
// (so the tab shows all); "" when it is.
func (s State) UnregisteredHint() string {
	if s.CwdProject != "" {
		return ""
	}
	return "not in a registered project, showing all; run `gx project add .`"
}

// PendingRowFor is the pending row for a ticket address, if it is queued.
func (s State) PendingRowFor(address string) (server.PendingRow, bool) {
	i := slices.IndexFunc(s.Pending, func(p server.PendingRow) bool { return p.Address == address })
	if i < 0 {
		return server.PendingRow{}, false
	}
	return s.Pending[i], true
}

// ToastFor is the toast a live event warrants. Only the caller's event path
// may use it: a snapshot or a reconnect re-snapshot describes state, not
// something that just happened, so it must never produce a toast.
func ToastFor(ev server.Event) (string, bool) {
	switch ev.Type {
	case server.EventIterationParked, server.EventTicketParked:
		return ev.Address + " parked", true
	}
	return "", false
}

// ApplySnapshot replaces the ticket rows and resets Seq. Live iteration state
// survives only for tickets still claimed on disk, since the snapshot itself
// does not describe iterations.
func (s State) ApplySnapshot(snap server.Snapshot) State {
	next := State{
		Seq:              snap.Seq,
		HerdrUnavailable: snap.HerdrUnavailable,
		Budget:           snap.Budget,
		ExtraUsage:       snap.ExtraUsage,
		Tickets:          slices.Clone(snap.Tickets),
		Queue:            s.Queue,
		Pending:          slices.Clone(snap.Pending),
		CwdProject:       s.CwdProject,
		AllProjects:      s.AllProjects,
	}
	for _, t := range next.Tickets {
		if it, ok := s.Iterations[t.Address]; ok && t.Status == statusClaimed {
			next = next.withIteration(t.Address, it)
		}
	}
	return next
}

// SetQueue replaces the queue with the given items' addresses.
func (s State) SetQueue(items []server.QueueItem) State {
	s.Queue = make([]string, len(items))
	for i, it := range items {
		s.Queue[i] = it.Address
	}
	return s
}

// Reduce applies one stream event. An event that is not exactly Seq+1 is not
// applied: a duplicate is dropped, a gap asks for a re-snapshot.
func (s State) Reduce(ev server.Event) (State, Effect) {
	switch {
	case ev.Seq <= s.Seq:
		return s, EffectNone
	case ev.Seq != s.Seq+1:
		return s, EffectResnapshot
	}
	s.Seq = ev.Seq

	switch ev.Type {
	case server.EventTicketClaimed:
		return s.withStatus(ev.Address, statusClaimed).withIteration(ev.Address, IterationClaimed), EffectNone
	case server.EventIterationStarted:
		return s.withIteration(ev.Address, IterationRunning), EffectNone
	case server.EventIterationParked:
		return s.withIteration(ev.Address, IterationParked), EffectNone
	case server.EventIterationFailed:
		return s.withIteration(ev.Address, IterationFailed), EffectNone
	case server.EventIterationLaunchFailed:
		// The claim was rolled back, so the on-disk status is not ours to guess.
		return s.withoutIteration(ev.Address), EffectResnapshot
	case server.EventTicketDone:
		return s.withStatus(ev.Address, statusDone).withoutIteration(ev.Address).withoutQueued(ev.Address), EffectNone
	case server.EventTicketCancelled:
		return s.withStatus(ev.Address, statusCancelled).withoutIteration(ev.Address).withoutQueued(ev.Address), EffectNone
	case server.EventQueueChanged:
		return s, EffectRefetchQueue
	case server.EventTicketChanged, server.EventTicketParked, server.EventReclaimed:
		return s, EffectResnapshot
	case server.EventHerdrUnavailable:
		s.HerdrUnavailable = true
	case server.EventHerdrAvailable:
		s.HerdrUnavailable = false
	}
	return s, EffectNone
}

const (
	statusClaimed   = "claimed"
	statusDone      = "done"
	statusCancelled = "cancelled"
)

func (s State) withStatus(address, status string) State {
	i := slices.IndexFunc(s.Tickets, func(t server.TicketInfo) bool { return t.Address == address })
	if i < 0 {
		return s
	}
	s.Tickets = slices.Clone(s.Tickets)
	s.Tickets[i].Status = status
	return s
}

func (s State) withIteration(address string, it IterationState) State {
	s.Iterations = maps.Clone(s.Iterations)
	if s.Iterations == nil {
		s.Iterations = map[string]IterationState{}
	}
	s.Iterations[address] = it
	return s
}

func (s State) withoutIteration(address string) State {
	if _, ok := s.Iterations[address]; !ok {
		return s
	}
	s.Iterations = maps.Clone(s.Iterations)
	delete(s.Iterations, address)
	return s
}

func (s State) withoutQueued(address string) State {
	s.Queue = slices.DeleteFunc(slices.Clone(s.Queue), func(a string) bool { return a == address })
	return s
}
