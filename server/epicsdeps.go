package server

import (
	"github.com/elentok/gx/server/epics"
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

		CapHeld:           func(root queue.RootRef, e tickets.Epic) (wire.Explanation, bool) { return s.explain.CapHeld(root, e) },
		AutoFFMerge:       func(project string) bool { return s.merger.AutoFFMerge(project) },
		CheckWaitingLabel: func(address string) string { return s.explain.CheckWaitingLabel(address) },
		AgentGone:         func(address string) (bool, error) { return s.ops.AgentGone(address) },
		AmbiguousBlockers: func(addr tickets.Address) []string { return s.ops.AmbiguousBlockers(addr) },
		DesignUncharted:   s.designs.Uncharted,
	}
}
