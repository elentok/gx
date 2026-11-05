package nativerunner

import (
	"errors"
	"fmt"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/harness"
)

// ptyTurn waits for a session's turn state: not busy, idle, a slash command's
// delivery and background tasks. It reads Pty's timing settings and failure
// mapping through p, as ptyPrompter does.
type ptyTurn struct{ p *Pty }

func (p *Pty) turns() ptyTurn { return ptyTurn{p} }

// promptSlash sends a "/" command once, never nudging or retyping: most
// commands start no turn, so a retype would run them twice. Delivered means
// Turn moved (a working report, a new sessionId, a dialog) or claude is still
// idle after N. A dialog command leaves the session blocked; gx never closes
// it.
func (t ptyTurn) promptSlash(pr *ptyPrompter, text string) error {
	if err := pr.Submit(text); err != nil {
		return err
	}
	started, err := pr.WaitStart(t.p.WaitN())
	if err != nil || started {
		return err
	}
	claude, _, program, err := pr.ss.holder.StatusWithProgram()
	if err != nil {
		return t.p.died(pr.ss, err)
	}
	if program == "working" || (claude != nil && claude.Status == ClaudeBusy) {
		return fmt.Errorf("%w: %q started no turn and claude is not idle", agentrunner.ErrNotDelivered, text)
	}
	return nil
}

// awaitNotBusy waits up to N for claude to stop being busy and returns the
// Turn to send against. A prompt typed into a busy claude is only queued, so
// its next idle edge would pass for delivery. A background task alone is not
// busy.
func (t ptyTurn) awaitNotBusy(ss *ptySession) (int, error) {
	p := t.p
	deadline := time.Now().Add(p.WaitN())
	for {
		resp, err := ss.holder.StatusReply()
		if err != nil {
			return 0, p.died(ss, err)
		}
		claude, turn, program := resp.Claude, resp.Turn, resp.Program
		// A watched harness (Codex) reports Background itself: working with a
		// background terminal takes a prompt, a running turn does not.
		if w := resp.WatchedView(); w != nil {
			switch {
			// After an error the input is ready: a prompt starts the next turn.
			case w.Status.State == harness.Blocked && w.Status.Blocked != harness.BlockedError:
				return 0, fmt.Errorf("%w: blocked on %s", agentrunner.ErrNotReady, w.Status.Message)
			case w.Status.State != harness.Working || w.Status.Background:
				return turn, nil
			}
			claude, program = nil, "working"
		}
		// Enter on a dialog would pick its first option, allowing a permission
		// prompt nobody answered.
		if claude != nil && claude.Status == ClaudeWaiting {
			return 0, fmt.Errorf("%w: blocked on %s", agentrunner.ErrNotReady, claude.WaitingFor)
		}
		// The OSC report leads the pid file by up to a poll.
		if program != "working" && (claude == nil || claude.Status != ClaudeBusy) {
			return turn, nil
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("%w: busy for %s", agentrunner.ErrNotReady, p.WaitN())
		}
		select {
		case <-ss.exited:
			return 0, p.died(ss, errors.New("holder exited"))
		case <-time.After(p.pollInterval()):
		}
	}
}

// awaitIdle waits up to N for claude to be neither busy nor blocked.
func (t ptyTurn) awaitIdle(ss *ptySession) error {
	p := t.p
	deadline := time.Now().Add(p.WaitN())
	for {
		resp, err := ss.holder.StatusReply()
		if err != nil {
			return p.died(ss, err)
		}
		claude, program := resp.Claude, resp.Program
		if w := resp.WatchedView(); w != nil {
			program = watchedProgram(w.Status)
		}
		busy := program == "working" || program == "blocked" ||
			(program == "" && claude != nil && (claude.Status == ClaudeBusy || claude.Status == ClaudeWaiting))
		if !busy {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: not idle after %s", agentrunner.ErrNotReady, p.WaitN())
		}
		select {
		case <-ss.exited:
			return p.died(ss, errors.New("holder exited"))
		case <-time.After(p.pollInterval()):
		}
	}
}

func (t ptyTurn) backgroundTasks(s agentrunner.Session) bool {
	p := t.p
	if p.BackgroundTasks != nil {
		return p.BackgroundTasks(s)
	}
	p.envProbeOnce.Do(func() {
		if home := envValue(p.Env, "HOME"); home != "" {
			p.envProbe = claudeui.BackgroundTasks(home, time.Now)
		}
	})
	return p.envProbe != nil && p.envProbe(s)
}
