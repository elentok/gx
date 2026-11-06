package budget

import (
	"fmt"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/server/wire"
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

// FollowUp is what a caller supplies to file a follow-up: the title and the
// text of each section, plus the key that makes two filings the same problem.
type FollowUp struct {
	Slug, Title, Source, Problem, SuggestedFix, Key string
}

func longTicketFollowUp(from tickets.Address, turns int) FollowUp {
	return FollowUp{
		Slug:    longTicketKey,
		Title:   "Implement sessions run for too many turns",
		Source:  fmt.Sprintf("Ticket %s reached %d assistant turns in one launch.", from, turns),
		Problem: "An implement session passed the long-ticket turn threshold, which usually means the ticket was too big or the agent is looping.",
		SuggestedFix: "Look at what the session spent its turns on. Split tickets of this size, " +
			"or fix the loop; raise budget.long-ticket-turns if the length is fine.",
		Key: longTicketKey,
	}
}

// GuardCallbacks wires the long-ticket alert into one iteration.
func (b *Budget) GuardCallbacks(addr tickets.Address, one *ralphloop.OneIteration) {
	one.LongTicketTurns = b.settings.Load().LongTicketTurns
	one.OnLongTicket = func(turns int) { b.alertLongTicket(addr, one.Ticket.Path, turns) }
}

// ClearLongTicketMark drops the previous launch's mark: each launch counts its
// turns anew, so a restarted or resumed ticket must not keep a stale badge.
func ClearLongTicketMark(ticketPath string) error {
	return schema.UpdateTicketWithBody(ticketPath, func(t *schema.Ticket, _ *string) { t.LongTicketTurns = 0 })
}

// alertLongTicket handles one launch's alert: it marks the ticket with the turn
// count, sends the notification and files or updates the follow-up. The session
// keeps running.
func (b *Budget) alertLongTicket(addr tickets.Address, ticketPath string, turns int) {
	if err := schema.UpdateTicketWithBody(ticketPath, func(t *schema.Ticket, _ *string) {
		t.LongTicketTurns = turns
	}); err != nil {
		b.log.Warn("mark long ticket", "ticket", addr.String(), "err", err)
	}
	b.events.Publish(wire.EventTicketChanged, addr.String())
	body := fmt.Sprintf("%s has run %d assistant turns", addr, turns)
	b.settings.Banner(wire.EventLevelWarning, "Long ticket", body)
	phrase := "long ticket " + addr.Epic + "/" + addr.ID
	if title := tickets.TicketTitle(ticketPath); title != "" {
		phrase += " " + title
	}
	b.chat.Notice(notify.ServerNotice{Kind: NoticeLongTicket, Emoji: "⏳", Title: phrase,
		Detail: fmt.Sprintf("The agent has run %d assistant turns.", turns)})
	if _, err := b.host.WriteFollowUp(addr, longTicketFollowUp(addr, turns)); err != nil {
		b.log.Warn("file long-ticket follow-up", "ticket", addr.String(), "err", err)
	}
}
