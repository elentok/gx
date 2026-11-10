package server

import (
	"time"

	"github.com/elentok/gx/server/epics"
	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// epicsDeps wires the epic read models to the root. The helpers that stay up
// are bound late, so a test that swaps a Server member is still seen.
func (s *Server) epicsDeps() epics.Deps {
	return epics.Deps{
		Log:         s.log,
		TicketStore: s.cfg.TicketStore,

		Harness:  s.harness,
		Projects: s.projects,
		Chat:     s.sessionChat,
		Settings: &s.settings,
		Writer:   s.writer,
		Runs:     s.registry,
		History:  s.idx,
		Queue:    s.queue,
		Pauses:   &s.pauseHold,
		Sessions: s.sessions,

		Workspaces: s.workspaces,

		CapHeld:     func(root queue.RootRef, e tickets.Epic) (wire.Explanation, bool) { return s.explain.CapHeld(root, e) },
		AutoFFMerge: func(project string) bool { return s.merger.AutoFFMerge(project) },
		Delivery:    func(project string) string { return s.merger.Delivery(project) },
		PRView:      s.PR.live,
		PRReason:    s.prReasons.Reason,
		PRActivity:  func(root queue.RootRef, e tickets.Epic) []wire.PRActivityEntry { return prflow.List(root.Project, e) },
		PRCIFix: func(e tickets.Epic) (time.Time, int) {
			since, _ := prflow.ResetTime(e.Path)
			perCheck, _ := prWatchHost{s}.CIFixCaps()
			return since, perCheck
		},
		CheckWaitingLabel: func(address string) string { return s.explain.CheckWaitingLabel(address) },
		AgentGone:         func(address string) (bool, error) { return s.ops.AgentGone(address) },
		LiveTask: func(address string) (epics.BackgroundTask, error) {
			live, stoppable, err := s.ops.LiveTask(address)
			return epics.BackgroundTask{Live: live, Stoppable: stoppable}, err
		},
		TaskCard:          func(address string) (*wire.BackgroundTaskCard, error) { return s.ops.TaskCard(address) },
		AmbiguousBlockers: func(addr tickets.Address) []string { return s.ops.AmbiguousBlockers(addr) },
		DesignUncharted:   s.designs.Uncharted,
	}
}
