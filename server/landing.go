package server

// Landing is the server's dirty-trunk landing and trunk-conflict code: the
// backup-and-restore land, the stash merge, the trunk-conflict finish and
// start-over, and the iteration mode choice. It is split from Server in place;
// the embedded *Server is the way into the rest of the server until the
// package move cuts it.
type Landing struct {
	*Server
}

func newLanding(s *Server) *Landing { return &Landing{Server: s} }
