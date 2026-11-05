package server

import (
	"slices"
	"strings"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// ticketRecommend stores an investigate agent's diagnosis and recommended verb
// on the paused ticket it was forked from. Only that ticket's own investigate
// child may call it, so an epic-level (deadlock) investigation, which has no
// parent, can never write one. The verb must be one detail offers right now.
func (s *Server) ticketRecommend(req wire.QueueRequest, qc queue.Ctx) (wire.QueueResult, error) {
	verb := strings.TrimSpace(req.Verb)
	diagnosis := strings.TrimSpace(req.Text)
	if verb == "" || diagnosis == "" {
		return queue.Refusal(wire.ReasonTextRequired, "a verb and a diagnosis are required"), nil
	}
	return s.writer.ResolvedWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		inv, ok, err := s.writer.FindTicket(qc.Investigator)
		if err != nil || !ok || inv.Ticket.Type != string(schema.TypeInvestigate) ||
			inv.Addr.Epic != ref.Addr.Epic || inv.Ticket.Parent == nil || *inv.Ticket.Parent != ref.Addr.ID {
			return queue.Refusal(wire.ReasonInvestigateStop, qc.Investigator+" is not the investigate ticket of "+ref.Addr.String()), nil
		}
		t, err := schema.ParseTicket(ref.Ticket.Path)
		if err != nil {
			return queue.Refusal(wire.ReasonInvestigateStop, "cannot read "+ref.Addr.String()+": "+err.Error()), nil
		}
		if t.PausedClass() == "" {
			return queue.Refusal(wire.ReasonNotPaused, ref.Addr.String()+" is not paused"), nil
		}
		evs, err := tickets.ReadLog(ref.Ticket.Path)
		if err != nil {
			return wire.QueueResult{}, err
		}
		actions, err := s.epics.DetailFor(ref.Addr.String(), t, tickets.Rounds(evs), nil)
		if err != nil {
			return wire.QueueResult{}, err
		}
		if !slices.Contains(actions, verb) {
			return queue.Refusal(wire.ReasonVerbNotOffered, verb+" is not an action of "+ref.Addr.String()), nil
		}
		written, err := tickets.RecommendOpenRound(ref.Ticket.Path, verb, diagnosis)
		if err != nil {
			return wire.QueueResult{}, err
		}
		if !written {
			return queue.Refusal(wire.ReasonNotPaused, ref.Addr.String()+" has no open pause round"), nil
		}
		s.idx.Rescan()
		return wire.QueueResult{}, nil
	})
}
