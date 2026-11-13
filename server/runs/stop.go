package runs

import (
	"errors"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/ticketops"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketlog"
)

// Stop is why gx is stopping a run: the pause kind it ends in, the detail
// the pause carries, and the reason that closes its open round.
type Stop struct {
	Kind        events.Kind
	Reason      string
	CloseReason string
}

type stopMark struct {
	stop   Stop
	paused bool
}

// Stopper stops a marked run's session and writes its pause. It is the one
// writer both the stopping path and the run's own settle use.
type Stopper struct {
	Registry *Registry
	// RunnerOf returns the runner that owns the run's session.
	RunnerOf func(Run) (agentrunner.Runner, error)
	// ProjectDir returns the scratch dir of a project.
	ProjectDir func(project string) (string, error)
	// PauseTicket is the root's one way to pause.
	PauseTicket func(scratchDir string, addr tickets.Address, ticketPath string, kind events.Kind, reason string, run Run) error
}

// SettleStop pauses t if it is marked stopped and this caller is the first to
// claim it. It reports whether t is being stopped, so a settling run can skip
// its own finish, merge and pause.
func (s *Stopper) SettleStop(t TrackedRun) (stopping bool, err error) {
	stop, stopping, first := s.Registry.ClaimStop(t.Address)
	if stopping && first {
		err = s.pause(t, stop)
	}
	return stopping, err
}

// pause stops the session through its runner, closes the open round and
// pauses the ticket.
func (s *Stopper) pause(t TrackedRun, stop Stop) error {
	addr, err := tickets.ParseAddress(t.Address, tickets.AddressContext{})
	if err != nil {
		return err
	}
	dir, err := s.ProjectDir(addr.Project)
	if err != nil {
		return err
	}
	r, closeErr := s.RunnerOf(t.Run)
	if closeErr == nil {
		closeErr = r.Stop(t.Session)
	}
	if err := ticketlog.EndOpenRound(t.TicketPath, ticketlog.SourceGx, ticketlog.HowAuto, stop.CloseReason); err != nil {
		closeErr = errors.Join(closeErr, err)
	}
	pauseErr := s.PauseTicket(dir, addr, t.TicketPath, stop.Kind, stop.Reason, t.Run)
	return errors.Join(closeErr, pauseErr)
}

// StormStop marks the run storm-stopped, then settles the stop: it stops the
// session and pauses the ticket process-storm with count and threshold in the
// detail. The watchdog's one entry point.
func (s *Stopper) StormStop(t TrackedRun, count, threshold int) error {
	s.MarkStorm(t.Address, count, threshold)
	_, err := s.SettleStop(t)
	return err
}

// MarkStorm marks the run storm-stopped without settling it, so a caller can
// kill the tree first and the run's own settle already finds the mark.
func (s *Stopper) MarkStorm(addr string, count, threshold int) {
	s.Registry.MarkStop(addr, Stop{
		Kind:        events.ProcessStorm,
		Reason:      schema.ProcessStormReason(count, threshold),
		CloseReason: ticketops.CloseProcessStorm,
	})
}

// BudgetStop is the marker for the budget hard limit.
var BudgetStop = Stop{
	Kind:        events.BudgetKilled,
	Reason:      "daily budget hard limit reached",
	CloseReason: ticketops.CloseBudgetKilled,
}
