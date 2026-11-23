package prompt

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/agentrunner/deliver"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/sessions/prompt/startclock"
	"github.com/elentok/gx/server/wire"
)

var (
	_ sessions.Prompter    = (*Prompt)(nil)
	_ sessions.FirstPrompt = (*firstPrompt)(nil)
)

const (
	// signalFirstIdle names the fallback in the debug line when no ready report came before the first Idle.
	signalFirstIdle = "first Idle"
	// holderPollInterval is how often a holder-backed prompt waits between turn checks.
	holderPollInterval = 50 * time.Millisecond
	// clearInputHint ends a reason when the input may still hold gx's text.
	clearInputHint = " Clear the input first."
)

var errNudgeCancelled = deliver.ErrCancelled

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
	clock       *startclock.StartClock
}

// NewFirstPrompt is sessions.Prompter's first-prompt tracker of one watch.
func (p *Prompt) NewFirstPrompt(dir, id string, run int) sessions.FirstPrompt {
	f := &firstPrompt{
		p: p, dir: dir, id: id, run: run, log: p.log.With("session", id),
		cancel: make(chan struct{}), turnStarted: make(chan struct{}),
		timeout: cmp.Or(p.cfg.FirstPromptStartTimeout, DefaultFirstPromptStartTimeout),
	}
	f.clock = startclock.NewStartClock(f.timeout, f.startTimedOut)
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
		f.releaseLaunchHold()
		f.raise(prompt, fmt.Sprintf("agent still starting after %s", f.timeout), wire.BlockingStarting)
	}
}

// releaseLaunchHold ends the launch hold of a prompt gx never typed, with an
// empty batch, so a person is not locked out of a session gx gave up on.
func (f *firstPrompt) releaseLaunchHold() {
	c, err := holder.DialHolder(f.dir)
	if err != nil {
		return
	}
	if err := c.Batch(func() error { return nil }); err != nil {
		f.log.Warn("release launch hold", "err", err)
	}
}

// raise logs the failure, then hands the prompt to a person as a one-step
// Action. A blocking kind lets Retry run it again.
func (f *firstPrompt) raise(prompt, reason, blocking string) {
	f.log.Warn("first prompt failed", "reason", reason)
	raiseBlocking(f.p, f.dir, reason, blocking, []string{prompt})
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
	a := deliveryCard(prompt, err)
	if a.Kind == wire.DeliverySendFailed {
		a.Reason = reason
	}
	f.log.Warn("first prompt failed", "reason", a.Reason)
	f.p.raiseActionable(f.dir, a)
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
	c, err := holder.DialHolder(f.dir)
	if err != nil {
		f.log.Warn("clear leftover first prompt", "err", err)
		return false, false
	}
	p := f.target(c, 0)
	if err := deliver.ClearInput(p, prompt); err != nil {
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
		if !f.typed.Load() {
			f.releaseLaunchHold()
		}
		// After delivery: the check dials the holder and runs `claude --version`,
		// and a marker that shows during delivery is not missing.
		f.logMissingMarker(signal)
		switch {
		case errors.Is(err, errNudgeCancelled):
			f.log.Debug(errNudgeCancelled.Error())
		case errors.Is(err, ErrDraft):
			f.log.Info("draft would not clear")
			f.raise(prompt, DraftReason, wire.BlockingDraft)
		case errors.Is(err, claudeui.ErrStuckSubmission), errors.Is(err, claudeui.ErrStartTimeout),
			errors.Is(err, claudeui.ErrNotReadyToType), errors.Is(err, errSessionBlocked),
			errors.Is(err, agentrunner.ErrNotDelivered):
			f.failed(prompt, err)
		case err != nil:
			f.log.Warn("type first prompt", "err", err)
		}
	})
}

// logMissingMarker logs one line, with the Claude version, when a signal other
// than the composer marker started the prompt and the marker was never seen:
// the version tells which release changed its composer.
func (f *firstPrompt) logMissingMarker(signal string) {
	c, err := holder.DialHolder(f.dir)
	if err != nil || signal == string(harness.ReadyComposer) {
		return
	}
	if seen, known, err := c.ComposerSeen(); err != nil || !known || seen {
		return
	}
	var read holder.ReadState
	if c.Supports(holder.OpScreenStyled) {
		_, read, _ = c.StyledScreenRead()
	}
	f.log.Info("first prompt started without the composer marker", "signal", signal, "claude_version", harness.ClaudeVersion(f.p.cfg.Env),
		"pty_size", read.PTY, "attach_sizes", read.Attaches)
	f.p.RaiseNotice(f.dir, wire.SessionNoticeNoComposerMarker)
}

func (f *firstPrompt) deliver(prompt string) error {
	c, err := holder.DialHolder(f.dir)
	if err != nil {
		return err
	}
	if f.vimRestarted(c, prompt) {
		return nil
	}
	// A slash prompt skips the Idle check on purpose: the ready report can arrive
	// while the session is still Starting, so only Blocked refuses.
	slash := strings.HasPrefix(prompt, "/")
	if w, err := c.Watched(); slash && err == nil && w != nil && w.Status.State == harness.Blocked {
		return errSessionBlocked
	}
	var source func() (string, bool, error)
	if slash {
		source = f.launchSource() // counts before the first send
	}
	_, turn, err := c.Status()
	if err != nil {
		return err
	}
	// Before the draft clear: vim NORMAL has to reach INSERT first. The nudge
	// runs the same step again, which presses nothing once in INSERT.
	// The first Submit inherits this lock, so the clear and the first type are one unit.
	unlock, err := lockTyping(f.p, f.id, f.cancel)
	if err != nil {
		return err
	}
	// The batch ends with the first Submit's Enter, or on any error return.
	release, err := deliver.HoldInput(c, unlock)
	if err != nil {
		return err
	}
	p := f.target(c, turn)
	f.firstImages().onto(p)
	p.Held = release
	defer p.ReleaseHeld()
	if !f.typing().ReadyToType(f.p.projects.TypingIO(f.dir, c, "first-prompt")) {
		return claudeui.ErrNotReadyToType
	}
	if f.prefillShown(p, prompt) {
		// The launch put the prompt in the input: clearing would erase it and
		// typing would double it. Enter alone sends it; a nudge that finds the
		// screen unchanged clears and types it in full.
		p.SkipType = true
	}
	// A swallowed first Enter is pressed again after seconds, not after the
	// grace: a person who started this chat is watching an empty page.
	policy := f.p.deliverPolicy(f.dir, prompt, cmp.Or(f.p.cfg.PromptGrace, deliver.DefaultNudgeCaps.Grace), "")
	policy.Draft = nil // the first prompt never clears a draft
	policy.Caps.Settle = min(sendSettle, policy.Caps.Grace)
	policy.Launch = f.launchCaps(policy.Slash)
	// A nudge means the first Enter did not start the turn. The pane at that
	// moment shows whether the text landed (Enter lost) or never did.
	typedAt := time.Now()
	policy.Report = func(d claudeui.NudgeDecision) {
		screen, _ := p.Screen()
		f.log.Info("first prompt nudge", "decision", string(d), "since_deliver", time.Since(typedAt).Round(time.Millisecond).String(), "screen", screen)
	}
	err = deliver.Deliver(p, prompt, policy)
	switch {
	case f.p.sessions.Watches.IsStopped() && err != nil:
		return nil
	case slash && source != nil && (err == nil || errors.Is(err, deliver.ErrMaybeUnsent)):
		if err != nil {
			f.log.Info("slash first prompt undecided; the launch proof decides", "err", err)
		}
		return f.proveDelivered(c, prompt, source)
	}
	return err
}

// launchCaps is the launch retype window: the config's LaunchTimings map onto
// it, and a Codex slash looks for Codex's marker.
func (f *firstPrompt) launchCaps(slash *deliver.SlashCaps) *deliver.LaunchCaps {
	lc := f.p.cfg.LaunchTimings.Caps()
	if slash != nil {
		lc.Marker = slash.Marker
	}
	return &lc
}

// vimRestarted runs the vim launch check for a claude session and, on a trip
// in a run that has not been restarted yet, hands the prompt to a fresh
// conversation. It reports true when it did, so the caller types nothing: the
// new watch's first prompt does. After the restart a trip is only logged (by
// the check) and the prompt is typed anyway, leaving a pane that hangs to the
// existing "prompt not delivered" Actionable.
func (f *firstPrompt) vimRestarted(c *holder.HolderClient, prompt string) bool {
	meta, err := sessions.ReadMeta(f.dir)
	if err != nil || (meta.Harness != "" && meta.Harness != string(harness.Claude)) {
		return false
	}
	if !harness.CheckVimLaunch(f.p.projects.TypingIO(f.dir, c, "first-prompt")) {
		return false
	}
	if meta.VimRelaunchRun != 0 && meta.VimRelaunchRun == f.run {
		return false
	}
	f.log.Warn("vim footer after launch, restarting once")
	f.p.resumer.RestartFresh(f.dir, f.id, prompt)
	return true
}

// errSessionBlocked means the first prompt was never typed because the session
// waits on a person.
var errSessionBlocked = errors.New("session is blocked")

// launchSource counts a claude session's person records now; nil for other harnesses.
func (f *firstPrompt) launchSource() func() (string, bool, error) {
	meta, err := sessions.ReadMeta(f.dir)
	if err != nil || (meta.Harness != "" && meta.Harness != string(harness.Claude)) {
		return nil
	}
	// Claude slugs its resolved cwd, so a symlinked session dir needs resolving.
	cwd := meta.Dir
	if real, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = real
	}
	path, err := f.p.projects.TranscriptPath(cwd, meta.ConversationID)
	if err != nil {
		f.log.Warn("first prompt proof skipped", "err", err)
		return nil
	}
	return claudeui.NewSessionRecordSource(meta.ConversationID, func() (string, bool, error) {
		_, err := os.Stat(path)
		return path, err == nil, nil
	})
}

// proveDelivered checks that a claude slash prompt reached its transcript. It
// only reads: Deliver did the typing and the retyping.
func (f *firstPrompt) proveDelivered(c *holder.HolderClient, prompt string, source func() (string, bool, error)) error {
	caps := f.p.cfg.LaunchTimings.ProofCaps()
	p := claudeui.LaunchProof{
		FirstUser:   source,
		CheckPasted: true,
		Pane: func() (string, error) {
			rows, _, err := c.Screen(0)
			return strings.Join(rows, "\n"), err
		},
		Now:   time.Now,
		Sleep: time.Sleep,
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
	err := claudeui.CheckLaunchDelivered(ctx, p, prompt, caps)
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

// typing is the typing rules of the session's harness.
func (f *firstPrompt) typing() harness.Typing {
	return f.p.projects.SessionTyping(f.dir)
}

// target is the first prompt's Target. A turn has started once the holder's
// Turn passes the one seen before typing.
func (f *firstPrompt) target(c *holder.HolderClient, turn int) *deliver.Target {
	return &deliver.Target{
		C: c, Typing: f.typing(), IO: f.p.projects.TypingIO(f.dir, c, "first-prompt-nudge"),
		Cancel: f.cancel, OnTyped: f.markTyped,
		Lock:  func() (func(), error) { return lockTyping(f.p, f.id, f.cancel) },
		Start: func(timeout time.Duration) (bool, error) { return f.waitStart(c, turn, timeout) },
	}
}

func (f *firstPrompt) waitStart(c *holder.HolderClient, turn int, timeout time.Duration) (bool, error) {
	deadline := time.After(timeout)
	for {
		_, now, err := c.Status()
		if err != nil {
			return false, err
		}
		if now > turn {
			return true, nil
		}
		select {
		case <-f.turnStarted:
			return true, nil
		case <-f.cancel:
			return false, errNudgeCancelled
		case <-f.p.sessions.Watches.Stop:
			return false, errors.New("server stopping")
		case <-deadline:
			return false, nil
		case <-time.After(holderPollInterval):
		}
	}
}

// PromptHead is the start of a prompt for a log line.
func PromptHead(prompt string) string {
	if r := []rune(prompt); len(r) > 40 {
		return string(r[:40]) + "…"
	}
	return prompt
}

// firstImages is the saved images the chat started with, pasted before the
// first prompt. A missing one is logged and skipped: the text still goes.
func (f *firstPrompt) firstImages() imageSet {
	meta, err := sessions.ReadMeta(f.dir)
	if err != nil || len(meta.FirstImages) == 0 {
		return imageSet{}
	}
	paths, refusal := f.p.resolveImages(f.dir, meta.FirstImages, true)
	if refusal != nil {
		f.log.Warn("first prompt images", "reason", refusal.Message)
		return imageSet{}
	}
	return f.p.imageSetOf(f.dir, meta.FirstImages, paths)
}

// prefillShown reports whether the launch passed prompt as --prefill and the
// input box now holds it. A launch that did not, or a box that does not show it
// (the flag was dropped, or the screen is not drawn yet), reads as no, and the
// prompt is typed.
func (f *firstPrompt) prefillShown(p *deliver.Target, prompt string) bool {
	meta, err := sessions.ReadMeta(f.dir)
	if err != nil || !meta.FirstPrefilled {
		return false
	}
	rows, _, err := p.ReadRows()
	if err != nil {
		return false
	}
	if !deliver.BoxHoldsText(rows, prompt, p.ImageCount()) {
		f.log.Warn("first prompt was prefilled but the input does not show it; typing it", "screen", strings.Join(rows, "\n"))
		return false
	}
	f.log.Info("first prompt prefilled; pressing Enter")
	return true
}
