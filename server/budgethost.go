package server

import (
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/tickets"
)

// budgetHost is budget.Host: what the budget kill reads from the root.
type budgetHost struct{ s *Server }

func (h budgetHost) PauseTicket(scratchDir string, addr tickets.Address, ticketPath string, kind events.Kind, reason string, run runs.Run) error {
	return h.s.pauseTicket(scratchDir, addr, ticketPath, kind, reason, run)
}
