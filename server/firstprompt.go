package server

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
)

// signalFirstIdle names the fallback in the debug line when no ready report
// came before the first Idle.
const signalFirstIdle = "first Idle"

// holderPollInterval is how often a holder-backed prompt waits between turn
// checks.
const holderPollInterval = 50 * time.Millisecond

var errNudgeCancelled = errors.New("nudge cancelled: person input")

// clearInputHint ends a reason when the input may still hold gx's text.
const clearInputHint = " Clear the input first."

// firstPrompt follows one session's first prompt from the watch stream: it
// decides when to type, types in the background, and cancels the remaining
// nudges when a person types too.
type firstPrompt struct {
	s           *Server
	dir         string
	log         *slog.Logger
	typed       atomic.Bool
	cancel      chan struct{}
	cancelOnce  sync.Once
	turnStarted chan struct{}
	turnOnce    sync.Once
	id          string
	run         int
	timeout     time.Duration
	clock       *startClock
}

// startClock counts only time spent Starting, so a dialog a person answers
// (Blocked) does not use it up. Only the watch loop touches it; the timer
// callback reads the session's state from its meta.
type startClock struct {
	timeout time.Duration
	timer   *time.Timer
	since   time.Time
	spent   time.Duration
}

// startStartClock starts the clock at launch; onTimeout fires when the
// session has spent timeout Starting.
func startStartClock(timeout time.Duration, onTimeout func()) *startClock {
	return &startClock{
		timeout: timeout,
		timer:   time.AfterFunc(timeout, onTimeout),
		since:   time.Now(),
	}
}

// track pauses the clock outside Starting and resumes it with what is left.
func (c *startClock) track(state string) {
	starting := state == SessionStarting || state == ""
	switch {
	case starting && c.since.IsZero():
		c.since = time.Now()
		c.timer.Reset(c.timeout - c.spent)
	case !starting && !c.since.IsZero():
		c.spent += time.Since(c.since)
		c.since = time.Time{}
		c.timer.Stop()
	}
}

func (c *startClock) stop() { c.timer.Stop() }

func (s *Server) newFirstPrompt(dir, id string, run int) *firstPrompt {
	f := &firstPrompt{
		s: s, dir: dir, id: id, run: run, log: s.log.With("session", id),
		cancel: make(chan struct{}), turnStarted: make(chan struct{}),
		timeout: cmp.Or(s.cfg.FirstPromptStartTimeout, DefaultFirstPromptStartTimeout),
	}
	f.clock = startStartClock(f.timeout, f.startTimedOut)
	return f
}

// stop ends the Starting clock when the watch ends.
func (f *firstPrompt) stop() { f.clock.stop() }

// startTimedOut hands a still-pending prompt to a person when the session is
// Starting at the deadline.
func (f *firstPrompt) startTimedOut() {
	var prompt string
	f.s.updateSessionMetaOrWarn(f.dir, f.id, func(m *sessionMeta) bool {
		starting := m.Status.State == "" || m.Status.State == SessionStarting
		if m.Run != f.run || m.FirstPrompt == "" || !starting {
			return false
		}
		prompt, m.FirstPrompt = m.FirstPrompt, ""
		return true
	})
	if prompt != "" {
		f.raise(prompt, fmt.Sprintf("agent still starting after %s", f.timeout))
	}
}

// raise logs the failure, then hands the prompt to a person as a one-step
// Action.
func (f *firstPrompt) raise(prompt, reason string) {
	f.log.Warn("first prompt failed", "reason", reason)
	f.s.raiseGxActionable(f.dir, reason, []string{prompt})
}

// failed raises the Actionable for a nudge that gave up. Where the input is
// not in vim NORMAL mode and was not just cleared, it first clears gx's own
// text once; a clear that cannot be confirmed tells the person to do it.
func (f *firstPrompt) failed(prompt string, err error) {
	reason, clearNeeded := failureReason(err)
	if clearNeeded {
		cleared, cancelled := f.clearLeftover(prompt)
		if cancelled {
			f.log.Debug(errNudgeCancelled.Error())
			return
		}
		if !cleared {
			reason = withClearHint(reason)
		}
	}
	f.raise(prompt, reason)
}

// failureReason words a first-prompt failure and says whether gx's own text
// may still be in the input and so needs the clear.
func failureReason(err error) (reason string, clearNeeded bool) {
	switch {
	case errors.Is(err, errSessionBlocked):
		return "session is blocked", false
	case errors.Is(err, claudeui.ErrNotReadyToType):
		return "input is in vim NORMAL mode", false
	case errors.Is(err, claudeui.ErrInputNotEmpty):
		return withClearHint("input not empty"), false
	case errors.Is(err, claudeui.ErrStartTimeout):
		return "agent never started", true
	}
	return "prompt never reached the input", true
}

func withClearHint(reason string) string {
	return reason + "." + clearInputHint
}

// clearLeftover clears the whole input once and reports whether the harness
// check confirms it empty.
func (f *firstPrompt) clearLeftover(prompt string) (cleared, cancelled bool) {
	c, err := nativerunner.DialHolder(f.dir)
	if err != nil {
		f.log.Warn("clear leftover first prompt", "err", err)
		return false, false
	}
	p := &holderPrompter{c: c, f: f}
	if err := claudeui.ClearInput(p, prompt); err != nil {
		if errors.Is(err, errNudgeCancelled) {
			return false, true
		}
		f.log.Warn("clear leftover first prompt", "err", err)
		return false, false
	}
	empty, known, err := p.InputEmpty()
	return err == nil && known && empty, false
}

// observe takes each status while the prompt is pending and returns the signal
// that lets it type: the ready report, else the first Idle. It never fires
// while Blocked.
func (f *firstPrompt) observe(state string, ready harness.ReadySignal) string {
	switch {
	case state == SessionBlocked:
		return ""
	case ready != "":
		return string(ready)
	case state == SessionIdle:
		return signalFirstIdle
	}
	return ""
}

// noteState closes turnStarted on the first Working after gx typed: the watch
// sees a turn start even where the holder's Turn counter does not move.
func (f *firstPrompt) noteState(state string) {
	f.clock.track(state)
	if state == SessionWorking && f.typed.Load() {
		f.turnOnce.Do(func() { close(f.turnStarted) })
	}
}

// personInput stops the remaining nudges once gx has typed the prompt.
func (f *firstPrompt) personInput() {
	if f.typed.Load() {
		f.cancelOnce.Do(func() { close(f.cancel) })
	}
}

func (f *firstPrompt) markTyped() {
	if f.typed.CompareAndSwap(false, true) {
		f.log.Debug("first prompt typed")
	}
}

// start types the prompt in the background, so the watch keeps reading the
// person's input.
func (f *firstPrompt) start(signal, prompt string) {
	f.log.Debug("first prompt ready", "signal", signal)
	f.s.sessionWatches.start(func() {
		err := f.deliver(prompt)
		switch {
		case errors.Is(err, errNudgeCancelled):
			f.log.Debug(errNudgeCancelled.Error())
		case errors.Is(err, errDraft):
			f.log.Info("draft would not clear")
			f.s.raiseGxActionable(f.dir, draftReason, []string{prompt})
		case errors.Is(err, claudeui.ErrStuckSubmission), errors.Is(err, claudeui.ErrStartTimeout),
			errors.Is(err, claudeui.ErrNotReadyToType), errors.Is(err, errSessionBlocked):
			f.failed(prompt, err)
		case err != nil:
			f.log.Warn("type first prompt", "err", err)
		}
	})
}

func (f *firstPrompt) deliver(prompt string) error {
	if strings.HasPrefix(prompt, "/") {
		return f.deliverSlash(prompt)
	}
	c, err := nativerunner.DialHolder(f.dir)
	if err != nil {
		return err
	}
	_, turn, err := c.Status()
	if err != nil {
		return err
	}
	// Before the draft clear: vim NORMAL has to reach INSERT first. The nudge
	// runs the same step again, which presses nothing once in INSERT.
	if !sessionTyping.ReadyToType(nativerunner.HolderTypingIO(c)) {
		return claudeui.ErrNotReadyToType
	}
	if err := f.s.clearDraft(f.dir, c); err != nil {
		return err
	}
	caps := claudeui.DefaultNudgeCaps
	caps.Grace = cmp.Or(f.s.cfg.FirstPromptGrace, caps.Grace)
	p := &holderPrompter{c: c, f: f, turn: turn}
	report := func(d claudeui.NudgeDecision) {
		f.log.Debug("first prompt nudge", "decision", string(d))
	}
	err = claudeui.PromptWithNudge(p, prompt, caps, time.Time{}, time.Now, report)
	if f.s.sessionWatches.isStopped() && err != nil {
		return nil
	}
	return err
}

// errSessionBlocked means the first prompt was never typed because the session
// waits on a person.
var errSessionBlocked = errors.New("session is blocked")

// deliverSlash types the prompt once and ends like an Action slash step. It
// skips the Idle check on purpose: the ready report can arrive while the
// session is still Starting, so only Blocked refuses. A step that fails after
// typing (blocked, ended) is only logged: the prompt was delivered.
func (f *firstPrompt) deliverSlash(prompt string) error {
	run, closeRun, err := f.s.openActionRun(f.dir, actionRunOpts{onTyped: f.markTyped, cancel: f.cancel})
	if err != nil {
		return err
	}
	defer closeRun()
	if err := run.catchUp(); err != nil {
		return err
	}
	if run.state == SessionBlocked {
		return errSessionBlocked
	}
	if err := run.step(0, prompt); err != nil {
		return err
	}
	select {
	case <-f.cancel:
		return errNudgeCancelled
	default:
		return nil
	}
}

// holderPrompter is claudeui.Prompter over a session's holder. A turn has
// started once the holder's Turn passes the one seen before typing.
type holderPrompter struct {
	c    *nativerunner.HolderClient
	f    *firstPrompt
	turn int
}

func (p *holderPrompter) cancelled() error {
	select {
	case <-p.f.cancel:
		return errNudgeCancelled
	default:
		return nil
	}
}

func (p *holderPrompter) Submit(text string) error {
	if err := p.cancelled(); err != nil {
		return err
	}
	if err := p.c.SendKeys(text); err != nil {
		return err
	}
	p.f.markTyped()
	return p.c.SendKeys("\r")
}

func (p *holderPrompter) Clear() error {
	if err := p.cancelled(); err != nil {
		return err
	}
	return p.c.SendKeys(sessionTyping.ClearKey())
}

func (p *holderPrompter) SendEnter() error {
	if err := p.cancelled(); err != nil {
		return err
	}
	return p.c.SendKeys("\r")
}

func (p *holderPrompter) WaitStart(timeout time.Duration) (bool, error) {
	deadline := time.After(timeout)
	for {
		_, turn, err := p.c.Status()
		if err != nil {
			return false, err
		}
		if turn > p.turn {
			return true, nil
		}
		select {
		case <-p.f.turnStarted:
			return true, nil
		case <-p.f.cancel:
			return false, errNudgeCancelled
		case <-p.f.s.sessionWatches.stop:
			return false, errors.New("server stopping")
		case <-deadline:
			return false, nil
		case <-time.After(holderPollInterval):
		}
	}
}

func (p *holderPrompter) Screen() (string, error) {
	rows, _, err := p.c.Screen(0)
	return strings.Join(rows, "\n"), err
}

func (p *holderPrompter) ReadyToType() bool {
	return sessionTyping.ReadyToType(nativerunner.HolderTypingIO(p.c))
}

// InputEmpty runs the harness empty check when the holder serves styled rows.
func (p *holderPrompter) InputEmpty() (bool, bool, error) {
	state, err := inputState(p.c)
	if err != nil {
		return false, false, err
	}
	switch state {
	case harness.InputEmpty:
		return true, true, nil
	case harness.InputNotEmpty:
		return false, true, nil
	}
	return false, false, nil
}
