package sched

import (
	"log/slog"

	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/projects"
	srvrecovery "github.com/elentok/gx/server/recovery"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/tickets"
)

// SchedulerDeps is what a Scheduler is built from.
type SchedulerDeps struct {
	Log      *slog.Logger
	Projects *projects.Registry
	Recovery *srvrecovery.Recovery
	Registry *runs.Registry
	Index    *index.Index
	Writer   *ticketwrite.Writer
	// TicketStore is the root of the ticket store.
	TicketStore string
}

// Scheduler watches the claim loop's surroundings: held background-task gates,
// epic deadlocks and finished claims nobody processed.
type Scheduler struct {
	SchedulerDeps
	deadlocks   deadlocks
	gatesRaised map[gateHold]bool // touched only by the gate watchdog
}

// NewScheduler returns a scheduler over d.
func NewScheduler(d SchedulerDeps) *Scheduler { return &Scheduler{SchedulerDeps: d} }

// AgentWaiting reports whether the run's agent is not working (idle or done).
// For tests.
func (s *Scheduler) AgentWaiting(project, epic, id string) bool {
	_, idle, err := s.Recovery.AgentState(tickets.Address{Project: project, Epic: epic, ID: id})
	return err == nil && idle
}
