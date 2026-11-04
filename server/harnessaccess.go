package server

import (
	harnesschoice "github.com/elentok/gx/server/harness"
)

// queueHost is queue.Host: the harness methods the queue reads, and the rest
// from the root.
type queueHost struct {
	*Server
	*harnesschoice.Harness
}

// Harness is the server's harness choice. Tests outside the package read it
// through here.
func (s *Server) Harness() *harnesschoice.Harness { return s.harness }
