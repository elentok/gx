package prompt

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/transcript"
)

var (
	_ sessions.Prompter    = (*Prompt)(nil)
	_ sessions.FirstPrompt = (*firstPrompt)(nil)
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
	p           *Prompt
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
	clock       *StartClock
}

// NewFirstPrompt is sessions.Prompter's first-prompt tracker of one watch.
func (p *Prompt) NewFirstPrompt(dir, id string, run int) sessions.FirstPrompt {
	f := &firstPrompt{
		p: p, dir: dir, id: id, run: run, log: p.log.With("session", id),
		cancel: make(chan struct{}), turnStarted: make(chan struct{}),
		timeout: cmp.Or(p.cfg.FirstPromptStartTimeout, DefaultFirstPromptStartTimeout),
	}
	f.clock = NewStartClock(f.timeout, f.startTimedOut)
	return f
}

// Stop ends the Starting clock when the watch ends.
func (f *firstPrompt) Stop() { f.clock.Stop() }

// startTimedOut hands a still-pending prompt to a person when the session is
// Starting at the deadline.
func (f *firstPrompt) startTimedOut() {
	var prompt string
	f.p.sessions.UpdateSessionMetaOrWarn(f.dir, f.id, func(m *sessions.Meta) bool {
		starting := m.Status.State == "" || m.Status.State == wire.SessionStarting
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
	f.p.RaiseGxActionable(f.dir, reason, []string{prompt})
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
	case errors.Is(err, agentrunner.ErrNotDelivered):
		return "prompt not delivered: " + strings.TrimPrefix(err.Error(), agentrunner.ErrNotDelivered.Error()+": "), true
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
	p := f.prompter(c, 0)
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

// Observe takes each status while the prompt is pending and returns the signal
// that lets it type: the ready report, else the first Idle. It never fires
// while Blocked.
func (f *firstPrompt) Observe(state string, ready harness.ReadySignal) string {
	switch {
	case state == wire.SessionBlocked:
		return ""
	case ready != "":
		return string(ready)
	case state == wire.SessionIdle:
		return signalFirstIdle
	}
	return ""
}

// NoteState closes turnStarted on the first Working after gx typed: the watch
// sees a turn start even where the holder's Turn counter does not move.
func (f *firstPrompt) NoteState(state string) {
	f.clock.Track(state)
	if state == wire.SessionWorking && f.typed.Load() {
		f.turnOnce.Do(func() { close(f.turnStarted) })
	}
}

// PersonInput stops the remaining nudges once gx has typed the prompt.
func (f *firstPrompt) PersonInput() {
	if f.typed.Load() {
		f.cancelOnce.Do(func() { close(f.cancel) })
	}
}

func (f *firstPrompt) markTyped() {
	if f.typed.CompareAndSwap(false, true) {
		f.log.Info("first prompt typed")
	}
}

// Start types the prompt in the background, so the watch keeps reading the
// person's input.
func (f *firstPrompt) Start(signal, prompt string) {
	f.log.Info("first prompt ready", "signal", signal, "len", len(prompt), "head", PromptHead(prompt))
	f.p.sessions.Watches.Start(func() {
		err := f.deliver(prompt)
		switch {
		case errors.Is(err, errNudgeCancelled):
			f.log.Debug(errNudgeCancelled.Error())
		case errors.Is(err, ErrDraft):
			f.log.Info("draft would not clear")
			f.p.RaiseGxActionable(f.dir, DraftReason, []string{prompt})
		case errors.Is(err, claudeui.ErrStuckSubmission), errors.Is(err, claudeui.ErrStartTimeout),
			errors.Is(err, claudeui.ErrNotReadyToType), errors.Is(err, errSessionBlocked),
			errors.Is(err, agentrunner.ErrNotDelivered):
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
	if !f.typing().ReadyToType(f.p.projects.TypingIO(f.dir, c, "first-prompt")) {
		return claudeui.ErrNotReadyToType
	}
	if err := f.p.ClearDraft(f.dir, c); err != nil {
		return err
	}
	caps := claudeui.DefaultNudgeCaps
	caps.Grace = cmp.Or(f.p.cfg.PromptGrace, caps.Grace)
	p := f.prompter(c, turn)
	report := func(d claudeui.NudgeDecision) {
		f.log.Debug("first prompt nudge", "decision", string(d))
	}
	err = claudeui.PromptWithNudge(p, prompt, caps, time.Time{}, time.Now, report)
	if f.p.sessions.Watches.IsStopped() && err != nil {
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
	run, closeRun, err := f.p.OpenActionRun(f.dir, RunOpts{OnTyped: f.markTyped, Cancel: f.cancel})
	if err != nil {
		return err
	}
	defer closeRun()
	if err := run.CatchUp(); err != nil {
		return err
	}
	if run.State() == wire.SessionBlocked {
		return errSessionBlocked
	}
	if err := run.Step(0, prompt); err != nil {
		return err
	}
	select {
	case <-f.cancel:
		return errNudgeCancelled
	default:
	}
	return f.proveDelivered(run, prompt)
}

// proveDelivered checks that a claude session's slash prompt reached its
// transcript, resending while it did not. Other harnesses write no such
// transcript, so they keep the unchecked send.
func (f *firstPrompt) proveDelivered(run ActionRun, prompt string) error {
	meta, err := sessions.ReadMeta(f.dir)
	if err != nil || (meta.Harness != "" && meta.Harness != string(harness.Claude)) {
		return nil
	}
	// Claude slugs its resolved cwd, so a symlinked session dir needs resolving.
	cwd, err := filepath.EvalSymlinks(meta.Dir)
	if err != nil {
		cwd = meta.Dir
	}
	path, err := f.p.projects.TranscriptPath(cwd, meta.ConversationID)
	if err != nil {
		f.log.Warn("first prompt proof skipped", "err", err)
		return nil
	}
	caps := claudeui.DefaultLaunchProofCaps
	caps.Grace = cmp.Or(f.p.cfg.LaunchProofCaps.Grace, caps.Grace)
	caps.UnknownGrace = cmp.Or(f.p.cfg.LaunchProofCaps.UnknownGrace, caps.UnknownGrace)
	caps.Poll = cmp.Or(f.p.cfg.LaunchProofCaps.Poll, caps.Poll)
	caps.ResendEvery = cmp.Or(f.p.cfg.LaunchProofCaps.ResendEvery, caps.ResendEvery)
	p := claudeui.LaunchProof{
		FirstUser: func() (string, bool, error) {
			msgs, _, err := transcript.ReadChat(path)
			if err != nil {
				return "", false, err
			}
			for _, m := range msgs {
				if m.Role == transcript.ChatRolePerson {
					return m.Text, true, nil
				}
			}
			return "", false, nil
		},
		Pane: func() (string, error) {
			rows, _, err := run.Holder().Screen(0)
			return strings.Join(rows, "\n"), err
		},
		Resend: func() error {
			select {
			case <-f.cancel:
				return errNudgeCancelled
			case <-f.p.sessions.Watches.Stop:
				return ErrServerStopping
			default:
			}
			return run.TypeStep(0, prompt)
		},
		Report: func(d claudeui.NudgeDecision) { f.log.Debug("first prompt nudge", "decision", string(d)) },
		Now:    time.Now,
		Sleep:  time.Sleep,
	}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go func() {
		select {
		case <-f.cancel:
		case <-f.p.sessions.Watches.Stop:
		case <-ctx.Done():
		}
		stop()
	}()
	err = claudeui.CheckLaunchDelivered(ctx, p, prompt, caps)
	if errors.Is(err, context.Canceled) {
		select {
		case <-f.cancel:
			return errNudgeCancelled
		default:
			return ErrServerStopping
		}
	}
	return err
}

// holderPrompter is the part of claudeui.Prompter every typed prompt shares:
// the keys and screen reads over a session's holder. Each kind of prompt adds
// its own WaitStart. Once cancel closes, no more keys go out; onTyped, when
// set, runs once the text is typed.
type holderPrompter struct {
	c       *nativerunner.HolderClient
	typing  harness.Typing
	io      harness.TypingIO
	cancel  <-chan struct{}
	onTyped func()
}

func (p *holderPrompter) cancelled() error {
	select {
	case <-p.cancel:
		return errNudgeCancelled
	default:
		return nil
	}
}

func (p *holderPrompter) Submit(text string) error {
	if err := p.cancelled(); err != nil {
		return err
	}
	if err := p.c.TypeTextBy(text, p.typing.PastesText(text)); err != nil {
		return err
	}
	if p.onTyped != nil {
		p.onTyped()
	}
	return p.submitKey()
}

// submitKey presses the harness submit key. Codex queues text with Tab while a
// turn runs; a background terminal alone is no turn.
func (p *holderPrompter) submitKey() error {
	working := false
	w, err := p.c.Watched()
	if err != nil {
		return err
	}
	if w != nil {
		working = w.Status.State == harness.Working && !w.Status.Background
	}
	return p.c.SendKeys(p.typing.SubmitKey(working))
}

func (p *holderPrompter) Clear() error {
	if err := p.cancelled(); err != nil {
		return err
	}
	return p.c.SendKeys(p.typing.ClearKey())
}

func (p *holderPrompter) SendEnter() error {
	if err := p.cancelled(); err != nil {
		return err
	}
	return p.submitKey()
}

// firstPrompter is the first prompt's Prompter. A turn has started once the
// holder's Turn passes the one seen before typing.
type firstPrompter struct {
	holderPrompter
	f    *firstPrompt
	turn int
}

// typing is the typing rules of the session's harness.
func (f *firstPrompt) typing() harness.Typing {
	return f.p.projects.SessionTyping(f.dir)
}

func (f *firstPrompt) prompter(c *nativerunner.HolderClient, turn int) *firstPrompter {
	return &firstPrompter{holderPrompter: holderPrompter{c: c, typing: f.typing(), io: f.p.projects.TypingIO(f.dir, c, "first-prompt-nudge"), cancel: f.cancel, onTyped: f.markTyped}, f: f, turn: turn}
}

func (p *firstPrompter) WaitStart(timeout time.Duration) (bool, error) {
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
		case <-p.f.p.sessions.Watches.Stop:
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

// Log is the server logger the clear's pane capture goes to.
func (p *holderPrompter) Log() *slog.Logger { return p.io.Log }

func (p *holderPrompter) ReadyToType() bool {
	return p.typing.ReadyToType(p.io)
}

// InputEmpty runs the harness empty check when the holder serves styled rows.
func (p *holderPrompter) InputEmpty() (bool, bool, error) {
	state, err := InputState(p.c)
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

// PromptHead is the start of a prompt for a log line.
func PromptHead(prompt string) string {
	if r := []rune(prompt); len(r) > 40 {
		return string(r[:40]) + "…"
	}
	return prompt
}
