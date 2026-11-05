package server

import (
	"errors"
	"fmt"
	"strings"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// agentVerb resolves a ticket to the runner session of its live iteration, so
// prompt, interrupt and answer reach the agent whichever runner hosts it.
func (s *Server) agentVerb(req wire.QueueRequest, do func(agentrunner.Runner, agentrunner.Session) (wire.QueueResult, error)) (wire.QueueResult, error) {
	return s.resolvedWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		runner, sess, ok, err := s.harness.LiveSession(ref.Addr)
		if err != nil {
			return wire.QueueResult{}, err
		}
		if !ok {
			return queue.Refusal(wire.ReasonIterationNotLive, "no live iteration for "+ref.Addr.String()), nil
		}
		return do(runner, sess)
	})
}

func (s *Server) agentPrompt(req wire.QueueRequest) (wire.QueueResult, error) {
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return queue.Refusal(wire.ReasonTextRequired, "text to send is required"), nil
	}
	return s.agentVerb(req, func(r agentrunner.Runner, sess agentrunner.Session) (wire.QueueResult, error) {
		err := r.Prompt(sess, text)
		if errors.Is(err, agentrunner.ErrNotReady) {
			return queue.Refusal(wire.ReasonAgentNotReady, err.Error()), nil
		}
		return wire.QueueResult{}, err
	})
}

func (s *Server) agentInterrupt(req wire.QueueRequest) (wire.QueueResult, error) {
	return s.agentVerb(req, func(r agentrunner.Runner, sess agentrunner.Session) (wire.QueueResult, error) {
		err := r.Interrupt(sess)
		if errors.Is(err, agentrunner.ErrNotReady) {
			return queue.Refusal(wire.ReasonAgentNotReady, err.Error()), nil
		}
		return wire.QueueResult{}, err
	})
}

// agentAnswer allows or denies the permission request a live agent waits on.
// With a request ID the open request must be that one and approvable; without
// one (the TUI and CLI) whatever is open is answered. A decision that reached
// the agent is logged as pause.permission; a refusal logs nothing.
func (s *Server) agentAnswer(req wire.QueueRequest) (wire.QueueResult, error) {
	decision := agentrunner.Decision(req.Decision)
	if decision != agentrunner.DecisionAllow && decision != agentrunner.DecisionDeny {
		return queue.Refusal(wire.ReasonBadDecision, "decision must be allow or deny"), nil
	}
	return s.resolvedWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		runner, sess, ok, err := s.harness.LiveSession(ref.Addr)
		if err != nil {
			return wire.QueueResult{}, err
		}
		if !ok {
			if req.RequestID != "" {
				return queue.Refusal(wire.ReasonAgentGone, "the agent is gone; a lost prompt cannot be resumed"), nil
			}
			return queue.Refusal(wire.ReasonIterationNotLive, "no live iteration for "+ref.Addr.String()), nil
		}
		answer := agentrunner.Answer{Decision: decision, RequestID: req.RequestID}
		var note string
		if decision == agentrunner.DecisionDeny {
			note, _ = tickets.ClipLogText(req.Text)
			answer.Text = note
		}
		if req.RequestID != "" && runner.Name() == string(agentrunner.ChoiceHerdr) {
			return queue.Refusal(wire.ReasonNotApprovable, "a herdr-hosted agent has no approval card"), nil
		}
		st, statusErr := runner.Status(sess)
		if req.RequestID != "" {
			if res, refused := checkOpenRequest(st, statusErr, req.RequestID); refused {
				return res, nil
			}
		}
		if err := runner.Answer(sess, answer); err != nil {
			return answerRefusal(err)
		}
		ev := tickets.LogEvent{Type: tickets.EventPausePermission, Decision: req.Decision, Source: req.Source, Note: note}
		if statusErr == nil {
			ev.Shown = permissionShown(st.Pending)
		}
		if _, err := tickets.AppendLog(ref.Ticket.Path, ev); err != nil {
			return wire.QueueResult{}, fmt.Errorf("log permission for %s: %w", ref.Addr, err)
		}
		if err := s.leaveBlocked(ref.Addr.String()); err != nil {
			return wire.QueueResult{}, err
		}
		// The prompt is gone, so open pages re-read the detail and the snapshot.
		s.events.Publish(wire.EventTicketChanged, ref.Addr.String())
		return wire.QueueResult{}, nil
	})
}

// agentAnswerWeb is agentAnswer for the web mux: the person answered a card, so
// the request must name the prompt they saw.
func (s *Server) agentAnswerWeb(req wire.QueueRequest) (wire.QueueResult, error) {
	if req.RequestID == "" {
		return queue.Refusal(wire.ReasonRequestIDRequired, "a request ID is required; review the current prompt"), nil
	}
	return s.agentAnswer(req)
}

// checkOpenRequest refuses unless st holds an approvable open request named id.
func checkOpenRequest(st agentrunner.Status, statusErr error, id string) (wire.QueueResult, bool) {
	switch {
	case errors.Is(statusErr, agentrunner.ErrNotFound), errors.Is(statusErr, agentrunner.ErrSessionDied):
		return queue.Refusal(wire.ReasonAgentGone, statusErr.Error()), true
	case statusErr != nil:
		return queue.Refusal(wire.ReasonNotBlocked, statusErr.Error()), true
	case st.State != agentrunner.StateBlocked:
		return queue.Refusal(wire.ReasonNotBlocked, "the agent is not waiting on a request"), true
	case st.Pending == nil || st.Pending.ID != id:
		return queue.Refusal(wire.ReasonRequestChanged, "the open request is not "+id), true
	case st.Pending.NotApprovable:
		return queue.Refusal(wire.ReasonNotApprovable, st.Pending.Tool+" needs a real answer, not an approval"), true
	}
	return wire.QueueResult{}, false
}

// answerRefusal maps a runner's Answer error to its refusal; others are errors.
func answerRefusal(err error) (wire.QueueResult, error) {
	switch {
	case errors.Is(err, agentrunner.ErrRequestMismatch):
		return queue.Refusal(wire.ReasonRequestChanged, err.Error()), nil
	case errors.Is(err, agentrunner.ErrNotFound), errors.Is(err, agentrunner.ErrSessionDied):
		return queue.Refusal(wire.ReasonAgentGone, err.Error()), nil
	case errors.Is(err, agentrunner.ErrNotDelivered):
		return queue.Refusal(wire.ReasonNotDelivered, err.Error()), nil
	case errors.Is(err, agentrunner.ErrNotReady):
		return queue.Refusal(wire.ReasonNotBlocked, "the agent is not waiting on a request"), nil
	case errors.Is(err, agentrunner.ErrInvalidDecision):
		return queue.Refusal(wire.ReasonBadDecision, "not a permission prompt; deny cancels it"), nil
	}
	return wire.QueueResult{}, err
}

// permissionShown is what the person saw of p, each text capped for the log.
func permissionShown(p *agentrunner.Pending) *tickets.PermissionShown {
	if p == nil {
		return nil
	}
	input, inputCut := tickets.ClipLogText(string(p.Input))
	desc, descCut := tickets.ClipLogText(p.Description)
	return &tickets.PermissionShown{RequestID: p.ID, Tool: p.Tool, Input: input, Description: desc, Truncated: inputCut || descCut}
}
