package server

import (
	"errors"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/tickets"
)

// PauseAs pauses a ticket as the given kind the way the server's own pauses do,
// including starting recovery. Tests in other packages call it, so it cannot
// live in an export_test.go.
func (s *Server) PauseAs(address string, kind events.Kind, reason string) error {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return err
	}
	ref, ok, err := s.FindTicket(address)
	if err != nil || !ok {
		return errors.Join(err, errors.New("no ticket "+address))
	}
	return s.writer.Pause(ref.ProjectDir, addr, ref.Ticket.Path, kind, reason, runs.Run{})
}
