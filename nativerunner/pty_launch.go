package nativerunner

import (
	"cmp"
	"errors"
	"fmt"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/harness"
)

var _ agentrunner.LaunchPrompter = (*Pty)(nil)

// PromptLaunch is PromptNudged for a launch prompt, which must start a turn.
// A Claude "/" command is typed once and then proved by the transcript: its
// first user record must be the command (see claudeui.CheckLaunchDelivered,
// which also owns the resends). A person typing over it is unconfirmed unless
// the transcript already shows the command. Other prompts, Codex, and runners
// without a Claude home keep the PromptNudged path.
func (p *Pty) PromptLaunch(s agentrunner.Session, text string, onNudge func(string)) error {
	home := envValue(p.Env, "HOME")
	ss, err := p.live(s)
	if err != nil {
		return err
	}
	if home == "" || (ss.kind != "" && ss.kind != harness.Claude) || !isSlashCommand(text) {
		return p.PromptNudged(s, text, onNudge)
	}
	turn, err := p.awaitNotBusy(ss)
	if err != nil {
		return err
	}
	pr := &ptyPrompter{p: p, ss: ss, turn: turn}
	if person, stop, err := p.PersonInput(s); err == nil {
		defer stop()
		pr.person = person
	}
	if err := pr.awaitEmptyInput(p.promptGrace()); err != nil {
		return notDelivered(err)
	}
	if err := pr.Submit(text); err != nil {
		return notDelivered(err)
	}
	proof := claudeui.LaunchProof{
		FirstUser: func() (string, bool, error) { return claudeui.FirstUserRecord(home, s.SessionID) },
		Pane: func() (string, error) {
			if err := pr.cancelled(); err != nil {
				return "", err
			}
			return pr.Screen()
		},
		Resend: func() error { return pr.Submit(text) },
		Report: func(d claudeui.NudgeDecision) {
			if onNudge != nil {
				onNudge(string(d))
			}
		},
		Now:   time.Now,
		Sleep: time.Sleep,
	}
	err = claudeui.CheckLaunchDelivered(proof, text, p.launchCaps())
	if errors.Is(err, errPersonInput) {
		// The person's keys end the resends; the transcript may still show the
		// command from before they typed.
		if ok, rerr := claudeui.LaunchRecorded(home, s.SessionID, text); rerr != nil || !ok {
			return fmt.Errorf("%w: %q: a person typed before the transcript showed the command", agentrunner.ErrDeliveryUnconfirmed, text)
		}
		return nil
	}
	if err != nil {
		return err
	}
	// The command is in the transcript; a launch prompt must also start a turn.
	started, err := pr.WaitStart(p.promptGrace())
	switch {
	case errors.Is(err, errPersonInput):
		return nil
	case err != nil:
		return err
	case !started:
		return fmt.Errorf("%w: %q started no turn", agentrunner.ErrNotDelivered, text)
	}
	return nil
}

// launchCaps is the launch proof's caps with LaunchCaps' set fields applied.
func (p *Pty) launchCaps() claudeui.LaunchProofCaps {
	caps := claudeui.DefaultLaunchProofCaps
	caps.Grace = cmp.Or(p.LaunchCaps.Grace, caps.Grace)
	caps.UnknownGrace = cmp.Or(p.LaunchCaps.UnknownGrace, caps.UnknownGrace)
	caps.Poll = cmp.Or(p.LaunchCaps.Poll, p.pollInterval())
	caps.ResendEvery = cmp.Or(p.LaunchCaps.ResendEvery, caps.ResendEvery)
	return caps
}
