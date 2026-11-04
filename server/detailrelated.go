package server

import (
	"github.com/elentok/gx/server/wire"
	"slices"
	"strings"

	"github.com/elentok/gx/tickets"
)

func (s *Server) detailStats(ref ticketRef) *wire.DetailStats {
	t := ref.ticket
	st := &wire.DetailStats{Elapsed: t.ElapsedTime, Cost: t.ActualCost, HadCodex: t.HadCodex, ContextWindow: t.ActualContextWindow}
	if at, ok := s.registry.startedAtOf(ref.addr.String()); ok && ref.epic.RenderedStatus(t) == tickets.StatusClaimed {
		st.ClaimedAt = at
	}
	return st
}

// relatedTickets lists one entry per blocked_by token, then the parent, then
// each fork child.
func relatedTickets(ref ticketRef, epics []tickets.Epic) []wire.RelatedTicket {
	own := func(t tickets.Ticket, relation string) wire.RelatedTicket {
		addr := tickets.Address{Project: ref.addr.Project, Epic: ref.addr.Epic, ID: t.DisplayNumber()}
		return wire.RelatedTicket{Relation: relation, Address: addr.String(), Linkable: true, Title: t.Title, Status: ref.epic.RenderedStatus(t).Word()}
	}
	related := []wire.RelatedTicket{}
	for _, token := range ref.ticket.BlockedBy {
		related = append(related, blockerEntry(ref, epics, token))
	}
	if ref.ticket.Parent != nil {
		if p, ok := findInEpic(ref.epic, *ref.ticket.Parent); ok {
			related = append(related, own(p, wire.RelationParent))
		}
	}
	for _, t := range ref.epic.Tickets {
		if t.Parent != nil && sameTicketID(*t.Parent, ref.ticket.DisplayNumber()) {
			related = append(related, own(t, wire.RelationFork))
		}
	}
	return related
}

// blockerEntry resolves one blocked_by token: bare ("04", "04a"), qualified
// ("epic/06") or a whole epic ("epic"). A whole epic reads done once it has
// merged and open before.
func blockerEntry(ref ticketRef, epics []tickets.Epic, token string) wire.RelatedTicket {
	entry := wire.RelatedTicket{Relation: wire.RelationBlocker, Address: token, Status: tickets.StatusError.Word()}
	epic, id := ref.epic, token
	if tickets.IsWholeEpicRef(token) {
		name := epicNameOf(token)
		for _, e := range epics {
			if e.Name == name {
				entry.Address = rootRef{Project: ref.addr.Project, Epic: name}.String()
				entry.Linkable = true
				entry.Title, entry.Status = e.Name, tickets.StatusOpen.Word()
				if !e.MergedAt.IsZero() {
					entry.Status = tickets.StatusDone.Word()
				}
			}
		}
		return entry
	}
	if name, rest, ok := strings.Cut(token, "/"); ok {
		id = rest
		i := slices.IndexFunc(epics, func(e tickets.Epic) bool { return e.Name == epicNameOf(name) })
		if i < 0 {
			return entry
		}
		epic = epics[i]
	}
	if t, ok := findInEpic(epic, id); ok {
		entry.Address = tickets.Address{Project: ref.addr.Project, Epic: epic.Name, ID: t.DisplayNumber()}.String()
		entry.Linkable = true
		entry.Title, entry.Status = t.Title, epic.RenderedStatus(t).Word()
	}
	return entry
}

// epicNameOf drops the optional "project:" prefix of an epic token.
func epicNameOf(token string) string {
	if _, name, ok := strings.Cut(token, ":"); ok {
		return name
	}
	return token
}

func findInEpic(e tickets.Epic, id string) (tickets.Ticket, bool) {
	for _, t := range e.Tickets {
		if sameTicketID(t.DisplayNumber(), id) {
			return t, true
		}
	}
	return tickets.Ticket{}, false
}

// sameTicketID compares ids ignoring zero-padding and case, so a token "4a"
// finds "04a".
func sameTicketID(a, b string) bool {
	norm := func(s string) string { return strings.ToLower(strings.TrimLeft(s, "0")) }
	return norm(a) == norm(b)
}
