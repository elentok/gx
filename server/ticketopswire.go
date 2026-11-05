package server

import (
	"net/http"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/ticketops"
	"github.com/elentok/gx/server/wire"
)

// newTicketOps needs s.writer, harness, registry, idx, events, epics and rec set.
func newTicketOps(s *Server) *ticketops.Ops {
	return ticketops.New(ticketops.Deps{
		Log: s.log, Writer: s.writer, Harness: s.harness, Registry: s.registry, Index: s.idx,
		Events: s.events, Epics: s.epics, Projects: s.projects, Rec: s.rec, Launch: ticketOpsLaunch{s},
	})
}

// ticketOpsLaunch is ticketops.Launch.
type ticketOpsLaunch struct{ s *Server }

func (l ticketOpsLaunch) BudgetRefusal() (wire.QueueResult, bool) { return l.s.budgetRefusal() }

func (l ticketOpsLaunch) PromptsOnRetry(address string) bool { return l.s.promptsOnRetry(address) }

func (l ticketOpsLaunch) RepairResume(web bool) http.HandlerFunc {
	return repairWriteAs(web, l.s.repairResume)
}

func (l ticketOpsLaunch) ResettleWaiting(t runs.TrackedRun) error { return l.s.resettleWaiting(t) }

// resettleWaiting ends a run's wait for an answer and settles it as a normal
// run: the run context is rebuilt from the registry entry, as a restart does.
func (s *Server) resettleWaiting(t runs.TrackedRun) error {
	runner, err := s.harness.RunnerOf(t.Run)
	if err != nil {
		return err
	}
	one, wt, mode, err := s.resume(t)
	if err != nil {
		return err
	}
	root, err := queue.ParseRootRef(t.Root)
	if err != nil {
		return err
	}
	s.registry.ClearWaiting(t.Address)
	deps := s.projects.DepsWith(runner)
	deps.GateReleased = s.registry.GateReleased(t.Address)
	s.guards.GoSettle(func() { s.finishRun(runContext{deps, root, mode, one, wt, t.Run, t.Address}) })
	return nil
}
