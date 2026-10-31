package server

import (
	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/conversations"
)

// sweepDesignCounts counts every Design conversation record, of either
// harness, that has no counts yet, so a session that never ended cleanly (a
// crash, or an exit while the server was down) still gets them. Records saved
// before the Design flag existed are Design when their ticket is on a map
// epic. A record whose transcript or rollout is missing stays uncounted; a
// later end recounts and replaces.
func (s *Server) sweepDesignCounts() {
	recs, err := conversations.List(s.cfg.ConversationsDir)
	if err != nil {
		s.log.Warn("sweep conversation counts", "err", err)
		return
	}
	for _, r := range recs {
		if r.Grilling != nil {
			continue
		}
		if !r.Design && !(r.Ticket != "" && s.onMapEpic(r.Ticket)) {
			continue
		}
		if err := s.recountGrilling(agentrunner.Kind(r.Harness), r.ID, r.Dir); err != nil {
			s.log.Warn("sweep conversation counts", "conversation", r.ID, "err", err)
		}
	}
}
