package server

import "github.com/elentok/gx/server/epics"

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

		AutoFFMerge:       func(project string) bool { return s.merger.AutoFFMerge(project) },
		CheckWaitingLabel: s.CheckWaitingLabel,
		AgentGone:         s.agentGone,
		AmbiguousBlockers: s.ambiguousBlockers,
		DesignUncharted:   s.designUncharted,
	}
}
