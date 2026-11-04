package server

import (
	"slices"
	"strings"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// ReasonVerbNotOffered refuses a recommendation of a verb outside detail's actions.
const ReasonVerbNotOffered = "verb-not-offered"

// ticketRecommend stores an investigate agent's diagnosis and recommended verb
// on the paused ticket it was forked from. Only that ticket's own investigate
// child may call it, so an epic-level (deadlock) investigation, which has no
// parent, can never write one. The verb must be one detail offers right now.
func (s *Server) ticketRecommend(req wire.QueueRequest, qc queueCtx) (wire.QueueResult, error) {
	verb := strings.TrimSpace(req.Verb)
	diagnosis := strings.TrimSpace(req.Text)
	if verb == "" || diagnosis == "" {
		return refusal(ReasonTextRequired, "a verb and a diagnosis are required"), nil
	}
	return s.resolvedWrite(req, func(ref ticketRef) (wire.QueueResult, error) {
		inv, ok, err := s.findTicket(qc.investigator)
		if err != nil || !ok || inv.ticket.Type != string(schema.TypeInvestigate) ||
			inv.addr.Epic != ref.addr.Epic || inv.ticket.Parent == nil || *inv.ticket.Parent != ref.addr.ID {
			return refusal(ReasonInvestigateStop, qc.investigator+" is not the investigate ticket of "+ref.addr.String()), nil
		}
		t, err := schema.ParseTicket(ref.ticket.Path)
		if err != nil {
			return refusal(ReasonInvestigateStop, "cannot read "+ref.addr.String()+": "+err.Error()), nil
		}
		if t.PausedClass() == "" {
			return refusal(ReasonNotPaused, ref.addr.String()+" is not paused"), nil
		}
		evs, err := tickets.ReadLog(ref.ticket.Path)
		if err != nil {
			return wire.QueueResult{}, err
		}
		actions, err := s.detailFor(ref.addr.String(), t, tickets.Rounds(evs), nil)
		if err != nil {
			return wire.QueueResult{}, err
		}
		if !slices.Contains(actions, verb) {
			return refusal(ReasonVerbNotOffered, verb+" is not an action of "+ref.addr.String()), nil
		}
		written, err := tickets.RecommendOpenRound(ref.ticket.Path, verb, diagnosis)
		if err != nil {
			return wire.QueueResult{}, err
		}
		if !written {
			return refusal(ReasonNotPaused, ref.addr.String()+" has no open pause round"), nil
		}
		s.rescan()
		return wire.QueueResult{}, nil
	})
}
