package sched

import (
	"log/slog"
	"time"

	"github.com/elentok/gx/server/budget"
	"github.com/elentok/gx/server/disk"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/merge"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/queue"
	srvrecovery "github.com/elentok/gx/server/recovery"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/tickets"
)

// ClaimExplainer is what the claim pass asks the explanation for: whether a
// rule holds a ticket back, and a flush of the changes the pass caused.
type ClaimExplainer interface {
	// ClaimHeld reports whether a claim hold names the ticket.
	ClaimHeld(project string, t tickets.Ticket, rh harnesschoice.ResolvedHarness) bool
	// PublishChanges streams the explanation changes since the last pass.
	PublishChanges()
}

// SchedulerDeps is what a Scheduler is built from.
type SchedulerDeps struct {
	Log      *slog.Logger
	Projects *projects.Registry
	Recovery *srvrecovery.Recovery
	Registry *runs.Registry
	Index    *index.Index
	Writer   *ticketwrite.Writer
	Settings *settings.Holder
	Migrator *projects.Migrator
	Queue    *queue.Queue
	Budget   *budget.Budget
	Disks    *disk.Disk
	Rec      *merge.Reconciler
	Merger   *merge.Merger
	Guards   *runs.Guards
	Harness  *harnesschoice.Harness
	Herdr    *HerdrWatch
	Explain  ClaimExplainer
	// Launcher is set once the launcher exists: it calls back into the
	// scheduler for the base rules.
	Launcher *Launcher
	// Env is the environment herdr is asked with.
	Env []string
	// PollInterval is the claim tick; zero means the index default.
	PollInterval time.Duration
	// TicketStore is the root of the ticket store.
	TicketStore string
}

// Scheduler runs the claim loop and watches its surroundings: held
// background-task gates, epic deadlocks and finished claims nobody processed.
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
