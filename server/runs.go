package server

import (
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/merge"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/sched"
	"github.com/elentok/gx/tickets"
)

// Runs lists the registry, for tests and later reads.
func (s *Server) Runs() []runs.Run { return s.registry.Runs() }

// Registry is the run registry, for tests that seed or read it.
func (s *Server) Registry() *runs.Registry { return s.registry }

// ResolveRun rebuilds the iteration a persisted handle stands for, whether or
// not its worktree is still on disk.
func (s *Server) ResolveRun(t runs.TrackedRun) (ralphloop.OneIteration, ralphloop.IterationWorktree, merge.IterationMode, error) {
	return s.reclaimer.ResolveRun(t)
}

// reclaimHost is sched.ReclaimHost.
type reclaimHost struct{ s *Server }

func (h reclaimHost) ConfiguredRunner() string { return h.s.cfg.Runner.Name() }

func (h reclaimHost) RunnerFor(run runs.Run) agentrunner.Runner { return h.s.harness.RunnerForRun(run) }

func (h reclaimHost) ProjectDir(project string) (string, error) {
	dir, _, err := h.s.projects.Of(project)
	return dir, err
}

func (h reclaimHost) Pause(dir string, addr tickets.Address, ticketPath string, kind events.Kind, reason string, run runs.Run) error {
	return h.s.writer.Pause(dir, addr, ticketPath, kind, reason, run)
}

func (h reclaimHost) StalledNudgeDelay() time.Duration { return h.s.stalledNudgeDelay() }

func (h reclaimHost) FinishSink(addr tickets.Address, ticketPath string) ralphloop.EventSink {
	return h.s.writer.FinishSink(addr, ticketPath)
}

func (h reclaimHost) IterationModeFor(addr tickets.Address, t tickets.Ticket) merge.IterationMode {
	return h.s.landing.IterationModeFor(addr, t)
}

func (h reclaimHost) DefaultDeps() ralphloop.Deps { return h.s.projects.DefaultDeps() }

func (h reclaimHost) Publish(event, id string) { h.s.events.Publish(event, id) }

func (h reclaimHost) ReportedFinished(ticketPath string) bool {
	return sched.ReportedFinished(ticketPath)
}

func (h reclaimHost) Continue(runner agentrunner.Runner, t runs.TrackedRun, root queue.RootRef, mode merge.IterationMode, one ralphloop.OneIteration, wt ralphloop.IterationWorktree, settleNow bool) {
	h.s.launcher.Continue(runner, t, root, mode, one, wt, settleNow)
}
