package server

import "github.com/elentok/gx/server/drafthistory"

// newDraftHistory builds the Draft history store. It is a free function so
// Server gains no method.
func newDraftHistory(s *Server) *drafthistory.Store {
	return drafthistory.New(s.cfg.StateDir)
}
