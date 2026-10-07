package server

import (
	"fmt"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// parkTicket is the one way the server parks a ticket: the write, the
// EventTicketParked event and the chat message all come from the same kind and
// reason, so the three cannot disagree.
func (s *Server) parkTicket(scratchDir string, addr tickets.Address, ticketPath string, kind events.Kind, reason string) error {
	if err := ralphloop.Park(scratchDir, addr.Epic, addr.ID, ticketPath, kind, reason); err != nil {
		return err
	}
	s.events.publish(EventTicketParked, addr.String())
	s.notifyPark(addr, ticketPath, kind, reason)
	return nil
}

// notifyPark sends the one chat message for a park that is already written.
// A park herdr's outage can cause is held for the digest instead.
func (s *Server) notifyPark(addr tickets.Address, ticketPath string, kind events.Kind, reason string) {
	status := string(schema.StatusNeedsAnswer)
	if kind.ParkType() == events.NeedsRepair {
		status = string(schema.StatusNeedsRepair)
	}
	if kind.CauseHerdr() && s.herdr.isUnavailable() {
		s.holdPark(addr, status, kind, reason)
		return
	}
	s.chat.Park(addr.Project, s.chatOverride(addr.Project), addr.Epic, ticketPath, addr.ID, status, fmt.Sprintf("%s: %s", kind, reason))
}
