package server

import (
	"errors"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/tickets"
)

// ClaimTicket claims and launches one ticket the way a claim pass would, but
// without asking the frontier, and reports whether an iteration launched.
// Tests in other packages call it, so it cannot live in an export_test.go.
func (s *Server) ClaimTicket(address, agent string) (bool, error) {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return false, err
	}
	ref, ok, err := s.writer.FindTicket(address)
	if err != nil || !ok {
		return false, errors.Join(err, errors.New("no ticket "+address))
	}
	epics, err := tickets.Load(ref.ProjectDir)
	if err != nil {
		return false, err
	}
	return s.launcher.ClaimAndLaunch(queue.RootOf(addr), addr, ref.Ticket, ref.Repo, ralphloop.AgentKind(agent), epics)
}
