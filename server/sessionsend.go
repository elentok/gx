package server

import (
	"cmp"
	"errors"
	"fmt"
	"github.com/elentok/gx/server/sessions"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/agentrunner/claudeui"
)

const sendFailedPrefix = "Send failed: "

// sendPrompt types a person's text into the session and nudges it until its
// turn starts, as the first prompt does: claude can swallow the Enter after a
// paste. A send that gives up raises a gx Actionable with the text, so the
// person can send it again.
func (s *Server) sendPrompt(dir, text string) {
	log := s.log.With("session", filepath.Base(dir))
	send := s.nudgePrompt
	if isSingleSendCommand(text) {
		send = s.typeCommandOnce
	}
	err := send(dir, text)
	switch {
	case err == nil:
		s.clearSendFailed(dir)
	case errors.Is(err, errNudgeCancelled), errors.Is(err, errServerStopping):
		log.Info("send stopped", "err", err)
	default:
		log.Warn("send failed", "err", err)
		s.raiseGxActionable(dir, fmt.Sprintf("%s%v", sendFailedPrefix, err), []string{text})
	}
}

// clearSendFailed drops an earlier "Send failed" card once a send gets
// through, so the person doesn't see a stale stop on a session that runs.
func (s *Server) clearSendFailed(dir string) {
	id := filepath.Base(dir)
	s.sessions.UpdateSessionMetaOrWarn(dir, id, func(m *sessions.Meta) bool {
		if !m.Actionable.GxRaised || !strings.HasPrefix(m.Actionable.Reason, sendFailedPrefix) {
			return false
		}
		m.Actionable = wire.SessionActionable{}
		return true
	})
}

// isSingleSendCommand reports whether text is a "/" command typed once.
// /compact, with or without arguments, starts a turn and keeps the nudge path.
// nativerunner/pty.go:isSlashCommand holds the runners' copy of this rule.
func isSingleSendCommand(text string) bool {
	if !strings.HasPrefix(text, "/") {
		return false
	}
	return text != "/compact" && !strings.HasPrefix(text, "/compact ")
}

// typeCommandOnce types a "/" command and presses Enter once: most commands
// start no turn, so a nudge would run them twice. A swallowed Enter leaves the
// text in the input box.
func (s *Server) typeCommandOnce(dir, text string) error {
	run, closeRun, err := s.openActionRun(dir, actionRunOpts{})
	if err != nil {
		return err
	}
	defer closeRun()
	if err := run.catchUp(); err != nil {
		return err
	}
	return run.typeStep(0, text)
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
	p := &runPrompter{holderPrompter: holderPrompter{c: run.holder, typing: s.projects.SessionTyping(dir), io: s.projects.TypingIO(dir, run.holder, "send-nudge")}, run: run}
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
