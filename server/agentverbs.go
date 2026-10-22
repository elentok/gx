package server

import (
	"errors"
	"fmt"
	"strings"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/tickets"
)

// Refusal reasons of the agent verbs. iteration-not-live is shared with nudge.
const (
	ReasonAgentNotReady = "agent-not-ready"
	ReasonNotBlocked    = "not-blocked"
	ReasonBadDecision   = "invalid-decision"
	// With a request ID: the open request differs, needs a real answer, or the agent is gone.
	ReasonRequestChanged = "request-changed"
	ReasonNotApprovable  = "not-approvable"
	// ReasonRequestIDRequired is the web's refusal of an answer that names no request.
	ReasonRequestIDRequired = "request-id-required"
	ReasonAgentGone         = "agent-gone"
	// ReasonAgentWaiting is the refusal of a plain resume on a live waiting
	// self-reported pause. It is public (ADR 0028) and named in the tracker text.
	ReasonAgentWaiting = "agent-waiting"
	// ReasonNotDelivered: a deny closed the dialog but its reason never reached the agent.
	ReasonNotDelivered = "reason-not-delivered"
)

// agentVerb resolves a ticket to the runner session of its live iteration, so
// prompt, interrupt and answer reach the agent whichever runner hosts it.
func (s *Server) agentVerb(req QueueRequest, do func(agentrunner.Runner, agentrunner.Session) (QueueResult, error)) (QueueResult, error) {
	return s.resolvedWrite(req, func(ref ticketRef) (QueueResult, error) {
		runner, sess, ok, err := s.liveSession(ref.addr)
		if err != nil {
			return QueueResult{}, err
		}
		if !ok {
			return refusal(ReasonIterationNotLive, "no live iteration for "+ref.addr.String()), nil
		}
		return do(runner, sess)
	})
}

func (s *Server) agentPrompt(req QueueRequest) (QueueResult, error) {
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return refusal(ReasonTextRequired, "text to send is required"), nil
	}
	return s.agentVerb(req, func(r agentrunner.Runner, sess agentrunner.Session) (QueueResult, error) {
		err := r.Prompt(sess, text)
		if errors.Is(err, agentrunner.ErrNotReady) {
			return refusal(ReasonAgentNotReady, err.Error()), nil
		}
		return QueueResult{}, err
	})
}

func (s *Server) agentInterrupt(req QueueRequest) (QueueResult, error) {
	return s.agentVerb(req, func(r agentrunner.Runner, sess agentrunner.Session) (QueueResult, error) {
		err := r.Interrupt(sess)
		if errors.Is(err, agentrunner.ErrNotReady) {
			return refusal(ReasonAgentNotReady, err.Error()), nil
		}
		return QueueResult{}, err
	})
}

// agentAnswer allows or denies the permission request a live agent waits on.
// With a request ID the open request must be that one and approvable; without
// one (the TUI and CLI) whatever is open is answered. A decision that reached
// the agent is logged as pause.permission; a refusal logs nothing.
func (s *Server) agentAnswer(req QueueRequest) (QueueResult, error) {
	decision := agentrunner.Decision(req.Decision)
	if decision != agentrunner.DecisionAllow && decision != agentrunner.DecisionDeny {
		return refusal(ReasonBadDecision, "decision must be allow or deny"), nil
	}
	return s.resolvedWrite(req, func(ref ticketRef) (QueueResult, error) {
		runner, sess, ok, err := s.liveSession(ref.addr)
		if err != nil {
			return QueueResult{}, err
		}
		if !ok {
			if req.RequestID != "" {
				return refusal(ReasonAgentGone, "the agent is gone; a lost prompt cannot be resumed"), nil
			}
			return refusal(ReasonIterationNotLive, "no live iteration for "+ref.addr.String()), nil
		}
		answer := agentrunner.Answer{Decision: decision, RequestID: req.RequestID}
		var note string
		if decision == agentrunner.DecisionDeny {
			note, _ = tickets.ClipLogText(req.Text)
			answer.Text = note
		}
		if req.RequestID != "" && runner.Name() == string(agentrunner.ChoiceHerdr) {
			return refusal(ReasonNotApprovable, "a herdr-hosted agent has no approval card"), nil
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
		ev := tickets.LogEvent{Type: tickets.EventParkPermission, Decision: req.Decision, Source: req.Source, Note: note}
		if statusErr == nil {
			ev.Shown = permissionShown(st.Pending)
		}
		if _, err := tickets.AppendLog(ref.ticket.Path, ev); err != nil {
			return QueueResult{}, fmt.Errorf("log permission for %s: %w", ref.addr, err)
		}
		// The prompt is gone, so open pages re-read the detail and the snapshot.
		s.events.publish(EventTicketChanged, ref.addr.String())
		return QueueResult{}, nil
	})
}

// agentAnswerWeb is agentAnswer for the web mux: the person answered a card, so
// the request must name the prompt they saw.
func (s *Server) agentAnswerWeb(req QueueRequest) (QueueResult, error) {
	if req.RequestID == "" {
		return refusal(ReasonRequestIDRequired, "a request ID is required; review the current prompt"), nil
	}
	return s.agentAnswer(req)
}

// checkOpenRequest refuses unless st holds an approvable open request named id.
func checkOpenRequest(st agentrunner.Status, statusErr error, id string) (QueueResult, bool) {
	switch {
	case errors.Is(statusErr, agentrunner.ErrNotFound), errors.Is(statusErr, agentrunner.ErrSessionDied):
		return refusal(ReasonAgentGone, statusErr.Error()), true
	case statusErr != nil:
		return refusal(ReasonNotBlocked, statusErr.Error()), true
	case st.State != agentrunner.StateBlocked:
		return refusal(ReasonNotBlocked, "the agent is not waiting on a request"), true
	case st.Pending == nil || st.Pending.ID != id:
		return refusal(ReasonRequestChanged, "the open request is not "+id), true
	case st.Pending.NotApprovable:
		return refusal(ReasonNotApprovable, st.Pending.Tool+" needs a real answer, not an approval"), true
	}
	return QueueResult{}, false
}

// answerRefusal maps a runner's Answer error to its refusal; others are errors.
func answerRefusal(err error) (QueueResult, error) {
	switch {
	case errors.Is(err, agentrunner.ErrRequestMismatch):
		return refusal(ReasonRequestChanged, err.Error()), nil
	case errors.Is(err, agentrunner.ErrNotFound), errors.Is(err, agentrunner.ErrSessionDied):
		return refusal(ReasonAgentGone, err.Error()), nil
	case errors.Is(err, agentrunner.ErrNotDelivered):
		return refusal(ReasonNotDelivered, err.Error()), nil
	case errors.Is(err, agentrunner.ErrNotReady):
		return refusal(ReasonNotBlocked, "the agent is not waiting on a request"), nil
	case errors.Is(err, agentrunner.ErrInvalidDecision):
		return refusal(ReasonBadDecision, "not a permission prompt; deny cancels it"), nil
	}
	return QueueResult{}, err
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
