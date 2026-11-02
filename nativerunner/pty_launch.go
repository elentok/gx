package nativerunner

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/transcript"
)

var _ agentrunner.LaunchPrompter = (*Pty)(nil)

// PromptLaunch is PromptNudged for a launch prompt, which must start a turn.
// A Claude "/" command is typed once and then proved by the transcript: its
// first user record must be the command (see claudeui.CheckLaunchDelivered,
// which also owns the resends). A person typing over it is unconfirmed unless
// the transcript already shows the command. Other prompts, Codex, and runners
// without a Claude home keep the PromptNudged path.
func (p *Pty) PromptLaunch(ctx context.Context, s agentrunner.Session, text string, onNudge func(string)) error {
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
	if err := pr.awaitReady(ctx, home); err != nil {
		return err
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

// DefaultReadyWait caps the wait for a launch prompt's ready signal.
const DefaultReadyWait = 30 * time.Second

// awaitReady waits for claude's ready signal: the input box on screen and the
// session transcript on disk. A missing signal is logged and the prompt goes
// anyway: the resend loop in CheckLaunchDelivered backstops a wrong signal, and
// a claude that never shows it must still get its prompt. A cancelled ctx
// returns at once with an error wrapping ctx.Err(), never the warning path.
func (pr *ptyPrompter) awaitReady(ctx context.Context, home string) error {
	p := pr.p
	wait := cmp.Or(p.ReadyWait, DefaultReadyWait)
	deadline := time.Now().Add(wait)
	rows := HolderTypingIO(pr.ss.holder).Rows
	for {
		screen, err := rows()
		if err != nil {
			return p.died(pr.ss, err)
		}
		files, err := transcript.FindByID(home, pr.ss.session.SessionID)
		if err == nil && len(files) == 1 && claudeui.InputBox(screen) {
			return nil
		}
		if !time.Now().Before(deadline) {
			if p.log != nil {
				p.log.Warn("ready signal missing, sending the launch prompt anyway", "session", pr.ss.session.SessionID, "waited", wait)
			}
			return nil
		}
		select {
		case <-pr.ss.exited:
			return p.died(pr.ss, errors.New("holder exited"))
		case <-ctx.Done():
			return fmt.Errorf("launch prompt cancelled: %w", context.Cause(ctx))
		case <-time.After(p.pollInterval()):
		}
	}
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
