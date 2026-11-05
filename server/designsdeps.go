package server

import "github.com/elentok/gx/server/designs"

// designsDeps reads the fields the design code copies; call it once they are set.
func (s *Server) designsDeps() designs.Deps {
	return designs.Deps{
		Log: s.log, TicketStore: s.cfg.TicketStore, ConversationsDir: s.cfg.ConversationsDir,
		Idx: s.idx, Projects: s.projects, Events: s.events, Guards: s.guards,
	}
}

// Designs gives tests the design seams (LiveDesignSessionID, RebaseDesign).
func (s *Server) Designs() *designs.Designs { return s.designs }
