package server

import (
	"errors"
	"fmt"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// modelMismatchKey is the follow-up's one dedupe key: every wrong-model pause
// is the same problem, so a repeat adds a "Seen again" line to the open draft.
const modelMismatchKey = "model-mismatch"

func modelMismatchFollowUp(from tickets.Address, m *ralphloop.ModelMismatchError) followUp {
	return followUp{
		Slug:    modelMismatchKey,
		Title:   "Implement sessions run on the wrong model",
		Source:  fmt.Sprintf("Ticket %s ran on %s but its tier resolved to %s, and was paused.", from, m.Ran, m.Expected),
		Problem: "An implement session's first assistant turn ran on a model other than the one gx launched it with.",
		SuggestedFix: "Find out why the launch did not get the requested model (a settings override, " +
			"an account default or a resumed session) and fix the cause, or drop the guard if the switch was deliberate.",
		Key: modelMismatchKey,
	}
}

// pauseModelMismatch settles a run whose session ran on the wrong model: it
// stops the session, keeps the branch and commits, pauses the ticket with both
// model names recorded, and files or updates the follow-up.
func (s *Server) pauseModelMismatch(rc runContext, addr tickets.Address, m *ralphloop.ModelMismatchError) {
	path, run := rc.one.Ticket.Path, rc.run
	var stopErr error
	if r, err := s.runnerOf(run); err != nil {
		stopErr = err
	} else {
		stopErr = r.Stop(run.Session)
	}
	if err := tickets.EndOpenRound(path, tickets.SourceGx, tickets.HowAuto, ralphloop.CloseModelMismatch); err != nil {
		stopErr = errors.Join(stopErr, err)
	}
	if err := schema.UpdateTicketWithBody(path, func(t *schema.Ticket, _ *string) {
		t.ModelRan, t.ModelExpected = m.Ran, m.Expected
	}); err != nil {
		stopErr = errors.Join(stopErr, err)
	}
	if stopErr != nil {
		s.log.Warn("model mismatch stop", "ticket", rc.ticketAddr, "err", stopErr)
	}
	if err := s.pauseTicket(rc.one.ScratchDir, addr, path, events.ModelMismatch, m.Error(), run); err != nil {
		s.log.Warn("pause model mismatch", "ticket", rc.ticketAddr, "err", err)
	}
	s.events.publish(EventIterationPaused, rc.ticketAddr)
	if _, err := s.writeFollowUp(addr, modelMismatchFollowUp(addr, m)); err != nil {
		s.log.Warn("file model-mismatch follow-up", "ticket", rc.ticketAddr, "err", err)
	}
}
