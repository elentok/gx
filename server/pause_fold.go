package server

import (
	"fmt"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
)

func (s *Server) flushPauseFold() {
	s.chat.PauseDigest(s.pauseFold.Flush())
}

// holdPause queues the pause line for the digest.
func (s *Server) holdPause(addr tickets.Address, status string, kind events.Kind, reason string) {
	s.pauseFold.Hold(fmt.Sprintf("[%s] %s/%s — %s (%s): %s", addr.Project, addr.Epic, addr.ID, status, kind, reason))
}
