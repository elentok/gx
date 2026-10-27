package server

import (
	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/grillround"
	"github.com/elentok/gx/harness"
)

// sweepDesignCounts counts every Design conversation record of the Claude
// harness that has no counts yet, so a session that never ended cleanly (a crash, or an exit while
// the server was down) still gets them. Records saved before the Design flag
// existed are Design when their ticket is on a map epic. A record whose
// transcript is missing stays uncounted; a later end recounts and replaces.
func (s *Server) sweepDesignCounts() {
	recs, err := conversations.List(s.cfg.ConversationsDir)
	if err != nil {
		s.log.Warn("sweep conversation counts", "err", err)
		return
	}
	home, err := s.envHome()
	if err != nil {
		s.log.Warn("sweep conversation counts", "err", err)
		return
	}
	for _, r := range recs {
		if r.Grilling != nil || r.Harness != string(harness.Claude) {
			continue
		}
		if !r.Design && !(r.Ticket != "" && s.onMapEpic(r.Ticket)) {
			continue
		}
		if err := grillround.Recount(s.cfg.ConversationsDir, home, r.ID, r.Dir); err != nil {
			s.log.Warn("sweep conversation counts", "conversation", r.ID, "err", err)
		}
	}
}
