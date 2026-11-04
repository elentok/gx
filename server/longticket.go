package server

import (
	"fmt"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

const (
	// longTicketKey is the follow-up's one dedupe key: every long session is the
	// same problem, so a repeat adds a "Seen again" line to the open draft.
	longTicketKey = "long-ticket"
	// NoticeLongTicket is the notify kind of the alert.
	NoticeLongTicket = "long-ticket"
)

func longTicketFollowUp(from tickets.Address, turns int) followUp {
	return followUp{
		Slug:    longTicketKey,
		Title:   "Implement sessions run for too many turns",
		Source:  fmt.Sprintf("Ticket %s reached %d assistant turns in one launch.", from, turns),
		Problem: "An implement session passed the long-ticket turn threshold, which usually means the ticket was too big or the agent is looping.",
		SuggestedFix: "Look at what the session spent its turns on. Split tickets of this size, " +
			"or fix the loop; raise budget.long-ticket-turns if the length is fine.",
		Key: longTicketKey,
	}
}

// guardCallbacks wires the long-ticket alert into one iteration.
func (s *Server) guardCallbacks(addr tickets.Address, one *ralphloop.OneIteration) {
	one.LongTicketTurns = s.settings.Load().LongTicketTurns
	one.OnLongTicket = func(turns int) { s.alertLongTicket(addr, one.Ticket.Path, turns) }
}

// clearLongTicketMark drops the previous launch's mark: each launch counts its
// turns anew, so a restarted or resumed ticket must not keep a stale badge.
func clearLongTicketMark(ticketPath string) error {
	return schema.UpdateTicketWithBody(ticketPath, func(t *schema.Ticket, _ *string) { t.LongTicketTurns = 0 })
}

// alertLongTicket handles one launch's alert: it marks the ticket with the turn
// count, sends the notification and files or updates the follow-up. The session
// keeps running.
func (s *Server) alertLongTicket(addr tickets.Address, ticketPath string, turns int) {
	if err := schema.UpdateTicketWithBody(ticketPath, func(t *schema.Ticket, _ *string) {
		t.LongTicketTurns = turns
	}); err != nil {
		s.log.Warn("mark long ticket", "ticket", addr.String(), "err", err)
	}
	s.events.publish(wire.EventTicketChanged, addr.String())
	body := fmt.Sprintf("%s has run %d assistant turns", addr, turns)
	s.banner(wire.EventLevelWarning, "Long ticket", body)
	s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeLongTicket, Emoji: "⏳", Title: "long ticket", Detail: body})
	if _, err := s.writeFollowUp(addr, longTicketFollowUp(addr, turns)); err != nil {
		s.log.Warn("file long-ticket follow-up", "ticket", addr.String(), "err", err)
	}
}
