package prompt

import (
	"cmp"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/agentrunner/claudeui"
)

const sendFailedPrefix = "Send failed: "

// sendPrompt types a person's text into the session and nudges it until its
// turn starts, as the first prompt does: claude can swallow the Enter after a
// paste. A send that gives up raises a gx Actionable with the text, so the
// person can send it again.
func (p *Prompt) sendPrompt(dir, text string, imgs imageSet) {
	log := p.log.With("session", filepath.Base(dir))
	send := p.nudgePrompt
	if isSingleSendCommand(text) {
		send = p.typeCommandOnce
	}
	err := send(dir, text, imgs)
	switch {
	case err == nil:
		p.clearSendFailed(dir)
	case errors.Is(err, errNudgeCancelled), errors.Is(err, ErrServerStopping):
		log.Info("send stopped", "err", err)
	default:
		log.Warn("send failed", "err", err)
		p.raiseActionable(dir, wire.SessionActionable{
			Reason: fmt.Sprintf("%s%v", sendFailedPrefix, err), Action: []string{text},
			Images: imgs.ids, GxRaised: true,
		})
	}
}

// clearSendFailed drops an earlier "Send failed" card once a send gets
// through, so the person doesn't see a stale stop on a session that runs.
func (p *Prompt) clearSendFailed(dir string) {
	id := filepath.Base(dir)
	p.sessions.UpdateSessionMetaOrWarn(dir, id, func(m *sessions.Meta) bool {
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
func (p *Prompt) typeCommandOnce(dir, text string, imgs imageSet) error {
	run, closeRun, err := p.openActionRun(dir, RunOpts{})
	if err != nil {
		return err
	}
	defer closeRun()
	if err := run.CatchUp(); err != nil {
		return err
	}
	return run.typeStepWith(0, text, imgs)
}

func (p *Prompt) nudgePrompt(dir, text string, imgs imageSet) error {
	run, closeRun, err := p.openActionRun(dir, RunOpts{})
	if err != nil {
		return err
	}
	defer closeRun()
	if err := run.CatchUp(); err != nil {
		return err
	}
	// Input from before gx typed is not a reason to stop.
	run.ClearPersonTyped()
	caps := claudeui.DefaultNudgeCaps
	caps.Grace = cmp.Or(p.cfg.PromptGrace, caps.Grace)
	pr := &runPrompter{holderPrompter: holderPrompter{c: run.Holder(), typing: p.projects.SessionTyping(dir), io: p.projects.TypingIO(dir, run.Holder(), "send-nudge"), images: imgs}, run: run}
	report := func(d claudeui.NudgeDecision) {
		p.log.Info("send nudge", "session", filepath.Base(dir), "decision", string(d))
	}
	return claudeui.PromptWithNudge(pr, text, caps, time.Time{}, time.Now, report)
}

// imageSet is the Prompt images a send pastes before its text. The zero value
// pastes nothing. paths[i] is the saved file of ids[i]; sent runs for an id
// once its own paste succeeds.
type imageSet struct {
	ids   []string
	paths []string
	sent  func(id string)
}

// paste pastes each image path as its own bracketed paste, in order.
func (s imageSet) paste(c interface{ Paste(string) error }) error {
	for i, path := range s.paths {
		if err := c.Paste(path); err != nil {
			return err
		}
		if s.sent != nil {
			s.sent(s.ids[i])
		}
	}
	return nil
}

// imageSetOf is the imageSet for resolved ids, marking each sent in dir as its
// paste succeeds.
func (p *Prompt) imageSetOf(dir string, ids, paths []string) imageSet {
	return imageSet{ids: ids, paths: paths, sent: func(id string) {
		if err := p.markImagesSent(dir, []string{id}); err != nil {
			p.log.Warn("marking image sent", "session", filepath.Base(dir), "image", id, "err", err)
		}
	}}
}

// withText is the text typed after the pastes: one space separates it from
// the last image's path.
func (s imageSet) withText(text string) string {
	if len(s.paths) == 0 {
		return text
	}
	return " " + text
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
		expired, _, err := p.run.NextStatus(timer)
		switch {
		case errors.Is(err, ErrWentBlocked):
			// A dialog opened, so the prompt was sent.
			return true, nil
		case err != nil:
			return false, err
		case p.run.PersonTyped():
			return false, errNudgeCancelled
		case p.run.TurnRunning():
			return true, nil
		case expired:
			return false, nil
		}
	}
}
