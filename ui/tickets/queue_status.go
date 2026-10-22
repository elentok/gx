package tickets

import (
	"slices"
	"time"

	"github.com/elentok/gx/server"
	gxtickets "github.com/elentok/gx/tickets"
	"github.com/elentok/gx/viewmodel"
)

// QueueStatus is the queue at a glance, for the title row: its mode and the
// done/total ticket counts of the epics it is working on.
type QueueStatus struct {
	// Mode is server.QueueModeRunning or QueueModePaused.
	Mode        server.QueueMode
	Done, Total int
}

// QueueStatusOf reads the queue's status from the server state; ok is false
// when there is nothing to show: no run is live and the queue is not held
// (paused) with work still in it.
func QueueStatusOf(vm viewmodel.State) (s QueueStatus, ok bool) {
	claimedAt := map[string]time.Time{}
	for _, t := range vm.Tickets {
		if !t.ClaimedAt.IsZero() {
			claimedAt[t.Address] = t.ClaimedAt
		}
	}
	running := false
	for _, epic := range epicsFromViewModel(vm, viewmodel.Scope{}) {
		live := epicRunning(epic, claimedAt)
		queued := slices.ContainsFunc(epic.Tickets, func(t gxtickets.Ticket) bool { return slices.Contains(vm.Queue, t.Path) })
		if !live && !queued {
			continue
		}
		running = running || live
		s.Done += epic.DoneCount()
		s.Total += epic.TotalCount()
	}
	s.Mode = vm.Mode
	if s.Mode == "" {
		s.Mode = server.QueueModeRunning
	}
	held := s.Mode != server.QueueModeRunning && len(vm.Queue) > 0
	return s, s.Total > 0 && (running || held)
}

// epicRunning reports whether the server holds a run for one of epic's
// tickets: a claimed ticket with a claim time.
func epicRunning(epic gxtickets.Epic, claimedAt map[string]time.Time) bool {
	return slices.ContainsFunc(epic.Tickets, func(t gxtickets.Ticket) bool {
		return serverRunning(epic, t, claimedAt)
	})
}

// serverRunning reports whether the server runs t. No claim time means the
// server holds no run for it: a person claimed it (a grilling session, say),
// so nothing is implementing.
func serverRunning(epic gxtickets.Epic, t gxtickets.Ticket, claimedAt map[string]time.Time) bool {
	if ticketStatus(epic, t) != gxtickets.StatusClaimed {
		return false
	}
	seen, ok := claimedAt[t.Path]
	return ok && !seen.IsZero()
}
