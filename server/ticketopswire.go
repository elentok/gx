package server

import (
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/ticketops"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// newTicketOps needs s.writer, harness, registry, idx, events, epics and rec set.
func newTicketOps(s *Server) *ticketops.Ops {
	return ticketops.New(ticketops.Deps{
		Writer: s.writer, Harness: s.harness, Registry: s.registry, Index: s.idx,
		Events: s.events, Epics: s.epics, Rec: s.rec, Landing: s.landing, Projects: s.projects,
		Log: s.log, TicketStore: s.cfg.TicketStore, Launch: ticketOpsLaunch{s},
	})
}

// FreshTicket loads addr's ticket, and its epic, straight from disk.
// It is the root's harness.Host method.
func (s *Server) FreshTicket(addr tickets.Address) (tickets.Epic, tickets.Ticket, bool) {
	return s.writer.FreshTicket(addr)
}

// HostingDeps talks to the runner hosting addr's iteration, so start-over and
// merge see (and close) the agent where it actually runs.
func (s *Server) HostingDeps(addr tickets.Address) deps.Deps {
	if s.cfg.Runner == nil {
		return s.projects.DefaultDeps()
	}
	// An unavailable runner is nil, which reports no session, as for an
	// unreachable host.
	runner, _ := s.harness.IterationRunner(addr)
	return s.projects.DepsWith(runner)
}

func (l ticketOpsLaunch) HostingDeps(addr tickets.Address) deps.Deps {
	return l.s.HostingDeps(addr)
}

// CompleteMergedRoot: a ticket of a queued root may be the one a merging
// paused, so once it merges the root completes as if its iteration had settled.
func (l ticketOpsLaunch) CompleteMergedRoot(addr tickets.Address) {
	s := l.s
	if !s.queue.RootQueued(queue.RootOf(addr)) {
		return
	}
	if _, t, ok := s.FreshTicket(addr); ok && s.guards.Merges.Begin() {
		defer s.guards.Merges.End()
		s.merger.CompleteRootIfDone(queue.RootOf(addr), ralphloop.OneIteration{Epic: addr.Epic, Ticket: t})
	}
}

// ticketOpsLaunch is ticketops.Launch.
type ticketOpsLaunch struct{ s *Server }

func (l ticketOpsLaunch) BudgetRefusal() (wire.QueueResult, bool) { return budgetRefusal(l.s) }

func (l ticketOpsLaunch) ResettleWaiting(t runs.TrackedRun) error { return resettleWaiting(l.s, t) }

// resettleWaiting ends a run's wait for an answer and settles it as a normal
// run: the run context is rebuilt from the registry entry, as a restart does.
func resettleWaiting(s *Server, t runs.TrackedRun) error {
	runner, err := s.harness.RunnerOf(t.Run)
	if err != nil {
		return err
	}
	one, wt, mode, err := s.reclaimer.Resume(t)
	if err != nil {
		return err
	}
	root, err := queue.ParseRootRef(t.Root)
	if err != nil {
		return err
	}
	s.registry.ClearWaiting(t.Address)
	s.launcher.Continue(runner, t, root, mode, one, wt, false)
	return nil
}
