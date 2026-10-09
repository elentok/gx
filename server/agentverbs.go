package server

import (
	"errors"
	"strings"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
)

// Refusal reasons of the agent verbs. iteration-not-live is shared with nudge.
const (
	ReasonAgentNotReady = "agent-not-ready"
	ReasonNotBlocked    = "not-blocked"
	ReasonBadDecision   = "invalid-decision"
)

// agentVerb resolves a ticket to the runner session of its live iteration, so
// prompt, interrupt and answer reach the agent whichever runner hosts it.
func (s *Server) agentVerb(req QueueRequest, do func(agentrunner.Runner, agentrunner.Session) (QueueResult, error)) (QueueResult, error) {
	return s.resolvedWrite(req, func(ref ticketRef) (QueueResult, error) {
		label, _, _ := ralphloop.IterationIdentity(ref.addr.Epic, ref.addr.ID, "")
		sess, ok, err := s.cfg.Runner.Find(label)
		if err != nil {
			return QueueResult{}, err
		}
		if !ok {
			return refusal(ReasonIterationNotLive, "no live iteration for "+ref.addr.String()), nil
		}
		return do(s.cfg.Runner, sess)
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
		return QueueResult{}, r.Interrupt(sess)
	})
}

func (s *Server) agentAnswer(req QueueRequest) (QueueResult, error) {
	decision := agentrunner.Decision(req.Decision)
	if decision != agentrunner.DecisionAllow && decision != agentrunner.DecisionDeny {
		return refusal(ReasonBadDecision, "decision must be allow or deny"), nil
	}
	return s.agentVerb(req, func(r agentrunner.Runner, sess agentrunner.Session) (QueueResult, error) {
		err := r.Answer(sess, agentrunner.Answer{Decision: decision, Text: req.Text})
		if errors.Is(err, agentrunner.ErrNotReady) {
			return refusal(ReasonNotBlocked, "the agent is not waiting on a request"), nil
		}
		return QueueResult{}, err
	})
}
