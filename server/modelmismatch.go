package server

import (
	"errors"
	"fmt"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// modelMismatchKey is the follow-up's one dedupe key: every wrong-model pause
// is the same problem, so a repeat adds a "Seen again" line to the open draft.
const modelMismatchKey = "model-mismatch"

// The model names recorded when a launch is refused for resolving to no model.
const (
	emptyModelRan      = "no model"
	emptyModelExpected = "a model from its tier"
)

func modelMismatchFollowUp(from tickets.Address, m *ralphloop.ModelMismatchError) followUp {
	source := fmt.Sprintf("Ticket %s ran on %s but its tier resolved to %s, and was paused.", from, m.Ran, m.Expected)
	if m.Ran == emptyModelRan {
		source = fmt.Sprintf("Ticket %s was paused before launch: its tier resolved to no model.", from)
	}
	return followUp{
		Slug:    modelMismatchKey,
		Title:   "Implement sessions run on the wrong model",
		Source:  source,
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
	var stopErr error
	if r, err := s.runnerOf(rc.run); err != nil {
		stopErr = err
	} else {
		stopErr = r.Stop(rc.run.Session)
	}
	s.recordModelMismatch(rc.one.ScratchDir, rc.one.Ticket.Path, addr, rc.run, m, stopErr)
}

// launchModel resolves the model and effort a launch runs on, honouring a
// one-shot tier override. ok is false when an implement launch resolved to no
// model: it is refused and paused, and no agent may start.
func (s *Server) launchModel(scratchDir string, t tickets.Ticket, addr tickets.Address, agent ralphloop.AgentKind) (model, effort string, ok bool) {
	var tierRun Run
	if tier := takeTierOverride(t.Path); tier != "" {
		tierRun.Model, tierRun.Effort = ralphloop.ResolveTier(s.agentsConfig(addr.Project), agent, tier)
	}
	model, effort = s.runAgentConfig(addr.Project, agent, tierRun)
	if model == "" && ralphloop.IsImplementType(t.Type) {
		s.refuseEmptyModel(scratchDir, t.Path, addr)
		return "", "", false
	}
	return model, effort, true
}

// refuseEmptyModel refuses an implement launch whose tier resolved to no model:
// the CLI would pick a default gx does not know. No session started, so there
// is nothing to stop.
func (s *Server) refuseEmptyModel(scratchDir, path string, addr tickets.Address) {
	m := &ralphloop.ModelMismatchError{Ran: emptyModelRan, Expected: emptyModelExpected}
	s.recordModelMismatch(scratchDir, path, addr, Run{}, m, nil)
}

// recordModelMismatch pauses the ticket with both model names recorded, notifies
// and files or updates the follow-up. stopErr is any error from stopping the
// session, logged with the rest.
func (s *Server) recordModelMismatch(scratchDir, path string, addr tickets.Address, run Run, m *ralphloop.ModelMismatchError, stopErr error) {
	ticketAddr := addr.String()
	if err := tickets.EndOpenRound(path, tickets.SourceGx, tickets.HowAuto, ralphloop.CloseModelMismatch); err != nil {
		stopErr = errors.Join(stopErr, err)
	}
	if err := schema.UpdateTicketWithBody(path, func(t *schema.Ticket, _ *string) {
		t.ModelRan, t.ModelExpected = m.Ran, m.Expected
	}); err != nil {
		stopErr = errors.Join(stopErr, err)
	}
	if stopErr != nil {
		s.log.Warn("model mismatch stop", "ticket", ticketAddr, "err", stopErr)
	}
	if err := s.pauseTicket(scratchDir, addr, path, events.ModelMismatch, m.Error(), run); err != nil {
		s.log.Warn("pause model mismatch", "ticket", ticketAddr, "err", err)
	}
	s.events.Publish(wire.EventIterationPaused, ticketAddr)
	if _, err := s.writeFollowUp(addr, modelMismatchFollowUp(addr, m)); err != nil {
		s.log.Warn("file model-mismatch follow-up", "ticket", ticketAddr, "err", err)
	}
}
