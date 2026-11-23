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
	"github.com/elentok/gx/agentrunner/codexui"
	"github.com/elentok/gx/agentrunner/deliver"
	"github.com/elentok/gx/harness"
)

// codexSlashWatch bounds the pane watch for Codex's "Unrecognized command",
// which Codex draws on screen and never writes to the rollout.
const codexSlashWatch = 2 * time.Second

// sendSettle is how long a chat send waits for its turn before it checks that the Enter landed.
const sendSettle = 3 * time.Second

const (
	sendFailedPrefix = "Send failed: "
	// maybeSentPrefix starts the card of a slash send whose outcome the screen
	// could not tell; it has no Action, as a resend could run the command twice.
	maybeSentPrefix = "May not have been sent: "
)

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
	policy := deliver.PolicyFor(text)
	policy.Draft = p.draftHooks(dir)
	policy.Caps.Grace = cmp.Or(p.cfg.PromptGrace, policy.Caps.Grace)
	// A person is waiting on a chat send: press a swallowed Enter after seconds, not after the grace.
	policy.Caps.Settle = min(sendSettle, policy.Caps.Grace)
	if policy.Slash != nil {
		policy.Slash.Settle = cmp.Or(p.cfg.ActionSlashSettle, DefaultActionSlashSettle)
		if meta, err := sessions.ReadMeta(dir); err == nil && meta.Harness == string(harness.Codex) {
			// One Enter, no input read: Deliver watches the pane for the marker after.
			policy.Slash.Settle = 0
			policy.Slash.Marker = codexui.UnknownCommandMarker
			policy.Slash.Watch = codexSlashWatch
		}
	}
	policy.Report = func(d claudeui.NudgeDecision) {
		p.log.Info("send nudge", "session", filepath.Base(dir), "decision", string(d))
	}
	id := filepath.Base(dir)
	t := &deliver.Target{C: run.Holder(), Typing: p.projects.SessionTyping(dir), IO: p.projects.TypingIO(dir, run.Holder(), "send"),
		Lock:  func() (func(), error) { return lockTyping(p, id, nil) },
		Start: func(timeout time.Duration) (bool, error) { return waitRunStart(run, timeout) }}
	imgs.onto(t)
	return deliver.Deliver(t, text, policy)
}

// finishSend settles a send's result: a success clears an earlier "Send failed"
// card, a stop raises nothing and any other error raises the card. A plain
// function, so Prompt's method count stays down.
func finishSend(p *Prompt, dir, text string, imgs imageSet, err error) {
	log := p.log.With("session", filepath.Base(dir))
	switch {
	case err == nil:
		p.clearSendFailed(dir)
	case errors.Is(err, errNudgeCancelled), errors.Is(err, ErrServerStopping):
		log.Info("send stopped", "err", err)
	case errors.Is(err, deliver.ErrMaybeUnsent):
		log.Warn("send undecided", "err", err)
		reason := strings.TrimPrefix(err.Error(), deliver.ErrMaybeUnsent.Error()+": ")
		p.raiseActionable(dir, wire.SessionActionable{Reason: maybeSentPrefix + reason, Images: imgs.ids, GxRaised: true})
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
		reason := m.Actionable.Reason
		if !m.Actionable.GxRaised || !strings.HasPrefix(reason, sendFailedPrefix) && !strings.HasPrefix(reason, maybeSentPrefix) {
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

// paste pastes each image path as its own bracketed paste, in order, waiting
// out the paste settle after each: an Enter sent while claude still ingests an
// image is swallowed, and the send then idles until the nudge grace ends.
func (s imageSet) paste(c interface{ TypeTextBy(string, bool) error }) error {
	for i, path := range s.paths {
		if err := c.TypeTextBy(path, true); err != nil {
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
