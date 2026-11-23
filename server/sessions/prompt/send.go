package prompt

import (
	"cmp"
	"errors"
	"path/filepath"
	"time"

	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/agentrunner/deliver"
)

// codexSlashWatch bounds the pane watch for Codex's "Unrecognized command",
// which Codex draws on screen and never writes to the rollout.
const codexSlashWatch = 2 * time.Second

// sendSettle is how long a chat send waits for its turn before it checks that the Enter landed.
const sendSettle = 3 * time.Second

// sendPrompt delivers a person's text to the session through deliver.Deliver. A
// send that gives up raises a gx Actionable with the text, so the person can
// send it again.
func (p *Prompt) sendPrompt(dir, text string, imgs imageSet) {
	finishSend(p, dir, text, imgs, p.deliverSend(dir, text, imgs))
}

// deliverSend opens the run and delivers text once, with the shape deliver.PolicyFor picks.
func (p *Prompt) deliverSend(dir, text string, imgs imageSet) error {
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
	policy := p.deliverPolicy(dir, text, cmp.Or(p.cfg.PromptGrace, deliver.DefaultNudgeCaps.Grace), "send nudge")
	// A person is waiting on a chat send: press a swallowed Enter after seconds, not after the grace.
	policy.Caps.Settle = min(sendSettle, policy.Caps.Grace)
	id := filepath.Base(dir)
	t := &deliver.Target{C: run.Holder(), Typing: p.projects.SessionTyping(dir), IO: p.projects.TypingIO(dir, run.Holder(), "send"),
		Lock:  func() (func(), error) { return lockTyping(p, id, nil) },
		Start: func(timeout time.Duration) (bool, error) { return waitRunStart(run, timeout) }}
	imgs.onto(t)
	return deliver.Deliver(t, text, policy)
}

// deliverPolicy is deliver.PolicyFor(text) with the draft hooks, the nudge
// grace and the slash settle of the config; report names the nudge log line.
func (p *Prompt) deliverPolicy(dir, text string, grace time.Duration, report string) deliver.Policy {
	policy := deliver.PolicyFor(text)
	policy.Draft = p.draftHooks(dir)
	policy.Caps.Grace = grace
	if policy.Slash != nil {
		kind := agentrunner.Kind("")
		if meta, err := sessions.ReadMeta(dir); err == nil {
			kind = agentrunner.Kind(meta.Harness)
		}
		caps := deliver.SlashCapsFor(kind, cmp.Or(p.cfg.ActionSlashSettle, DefaultActionSlashSettle), deliver.DefaultSlashCaps.Gap, codexSlashWatch)
		policy.Slash = &caps
	}
	policy.Report = func(d claudeui.NudgeDecision) {
		p.log.Info(report, "session", filepath.Base(dir), "decision", string(d))
	}
	return policy
}

// finishSend settles a send's result: a success clears an earlier delivery
// card, a stop raises nothing and any other error raises the card. A plain
// function, so Prompt's method count stays down.
func finishSend(p *Prompt, dir, text string, imgs imageSet, err error) {
	log := p.log.With("session", filepath.Base(dir))
	switch {
	case err == nil:
		p.clearSendFailed(dir)
	case errors.Is(err, errNudgeCancelled), errors.Is(err, ErrServerStopping):
		log.Info("send stopped", "err", err)
	default:
		log.Warn("send failed", "err", err)
		a := deliveryCard(text, err)
		a.Images = imgs.ids
		p.raiseActionable(dir, a)
	}
}

// clearSendFailed drops an earlier delivery card once a send gets
// through, so the person doesn't see a stale stop on a session that runs.
func (p *Prompt) clearSendFailed(dir string) {
	id := filepath.Base(dir)
	p.sessions.UpdateSessionMetaOrWarn(dir, id, func(m *sessions.Meta) bool {
		if !m.Actionable.GxRaised || !m.Actionable.IsDelivery() {
			return false
		}
		m.Actionable = wire.SessionActionable{}
		return true
	})
}

// imageSet is the Prompt images a send pastes before its text. The zero value
// pastes nothing. paths[i] is the saved file of ids[i]; sent runs for an id
// once its own paste succeeds.
type imageSet struct {
	ids   []string
	paths []string
	sent  func(id string)
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

// onto sets the images the Target pastes before its text.
func (s imageSet) onto(t *deliver.Target) {
	t.Images = s.paths
	if s.sent != nil {
		t.ImageSent = func(i int) { s.sent(s.ids[i]) }
	}
}

// waitRunStart is a send's start signal: the run's watch tells when the turn
// starts, and a person's own input stops the nudges.
func waitRunStart(run *actionRun, timeout time.Duration) (bool, error) {
	timer := time.After(timeout)
	for {
		expired, _, err := run.NextStatus(timer)
		switch {
		case errors.Is(err, ErrWentBlocked):
			// A dialog opened, so the prompt was sent.
			return true, nil
		case err != nil:
			return false, err
		case run.PersonTyped():
			return false, errNudgeCancelled
		case run.TurnRunning():
			return true, nil
		case expired:
			return false, nil
		}
	}
}
