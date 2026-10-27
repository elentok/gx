package server

import (
	"cmp"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
)

// sendPrompt types a person's text into the session and nudges it until its
// turn starts, as the first prompt does: claude can swallow the Enter after a
// paste. A send that gives up raises a gx Actionable with the text, so the
// person can send it again.
func (s *Server) sendPrompt(dir, text string) {
	log := s.log.With("session", filepath.Base(dir))
	err := s.nudgePrompt(dir, text)
	switch {
	case err == nil:
	case errors.Is(err, errNudgeCancelled), errors.Is(err, errServerStopping):
		log.Info("send stopped", "err", err)
	default:
		log.Warn("send failed", "err", err)
		s.raiseGxActionable(dir, fmt.Sprintf("Send failed: %v", err), []string{text})
	}
}

func (s *Server) nudgePrompt(dir, text string) error {
	run, closeRun, err := s.openActionRun(dir, actionRunOpts{})
	if err != nil {
		return err
	}
	defer closeRun()
	if err := run.catchUp(); err != nil {
		return err
	}
	// Input from before gx typed is not a reason to stop.
	run.personTyped = false
	caps := claudeui.DefaultNudgeCaps
	caps.Grace = cmp.Or(s.cfg.PromptGrace, caps.Grace)
	p := &runPrompter{holderPrompter: holderPrompter{c: run.holder, typing: run.s.typing()}, run: run}
	report := func(d claudeui.NudgeDecision) {
		s.log.Info("send nudge", "session", filepath.Base(dir), "decision", string(d))
	}
	return claudeui.PromptWithNudge(p, text, caps, time.Time{}, time.Now, report)
}

// runPrompter is a send's Prompter: the run's watch tells when the turn
// starts, and a person's own input stops the nudges.
type runPrompter struct {
	holderPrompter
	run *actionRun
}

func (p *runPrompter) WaitStart(timeout time.Duration) (bool, error) {
	timer := time.After(timeout)
	for {
		expired, _, err := p.run.nextStatus(timer)
		switch {
		case errors.Is(err, errWentBlocked):
			// A dialog opened, so the prompt was sent.
			return true, nil
		case err != nil:
			return false, err
		case p.run.personTyped:
			return false, errNudgeCancelled
		case p.run.turnRunning():
			return true, nil
		case expired:
			return false, nil
		}
	}
}
