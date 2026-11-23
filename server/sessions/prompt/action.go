package prompt

import (
	"cmp"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/agentrunner/deliver"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
)

// RunSessionAction clears an Idle session's Actionable and sends its Action
// in the background, so the run outlives the client that asked for it.
func (p *Prompt) RunSessionAction(req wire.SessionRequest) (wire.SessionResult, error) {
	return p.SendSessionAction(req, false)
}

// SendSessionAction is RunSessionAction; auto marks a send the server made
// itself, which the runaway guard counts. Any other send is the person's input
// and resets that count.
func (p *Prompt) SendSessionAction(req wire.SessionRequest, auto bool) (wire.SessionResult, error) {
	p.sessions.Watches.MetaMu.Lock()
	defer p.sessions.Watches.MetaMu.Unlock()
	dir, meta, res, err := p.sessions.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if st := meta.Info().Status; !st.Promptable() {
		return sessions.Refusal(wire.ReasonSessionNotIdle, fmt.Sprintf("session %s is %s, not idle", req.ID, st.State)), nil
	}
	action := meta.Actionable.Action
	if len(action) == 0 {
		return sessions.Refusal(wire.ReasonSessionNoAction, fmt.Sprintf("session %s has no action", req.ID)), nil
	}
	if res, refused := p.resumer.HandoffRefusal(meta, action); refused {
		return res, nil
	}
	// A failed send's images go back through the prompt route's send.
	var imgs imageSet
	if ids := meta.Actionable.Images; len(ids) > 0 && len(action) == 1 {
		paths, refusal := p.resolveImages(dir, ids, true)
		if refusal != nil {
			return *refusal, nil
		}
		imgs = p.imageSetOf(dir, ids, paths)
	}
	meta.Actionable = wire.SessionActionable{}
	if auto {
		meta.AutoSends++
	} else {
		p.CancelAutoAdvance(req.ID, "Auto-advance stopped: you ran the action")
		meta.AutoSends = 0
	}
	if err := sessions.WriteMeta(dir, meta); err != nil {
		return wire.SessionResult{}, err
	}
	p.events.Publish(wire.EventSessionChanged, req.ID)
	if text := meta.Actionable.CopyText; text != "" && meta.Actionable.IsDelivery() {
		// Through the chat send path, so a second failure raises the same card.
		p.sessions.Watches.Start(func() { p.sendPrompt(dir, text, imgs) })
	} else if len(imgs.ids) > 0 {
		p.sessions.Watches.Start(func() { p.sendPrompt(dir, action[0], imgs) })
	} else {
		p.resumer.StartContinue(dir, meta, "Action", action)
	}
	info := meta.Info()
	return wire.SessionResult{Session: &info}, nil
}

// retryBlocking is the Retry button of a blocking gx card: it runs the unsent
// steps as `a` does, whose first step clears the draft. It does not restart the
// first-prompt path or wait for the first Idle. A plain function, so Prompt's
// method count stays down.
func retryBlocking(p *Prompt, req wire.SessionRequest) (wire.SessionResult, error) {
	_, meta, res, err := p.sessions.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	a := meta.Actionable
	if !a.GxRaised || (a.Blocking == "" && a.Kind != wire.DeliveryUnknownCommand) {
		return sessions.Refusal(wire.ReasonSessionNoAction, fmt.Sprintf("session %s has no blocking card", req.ID)), nil
	}
	if meta.Status.State == wire.SessionExited {
		return sessions.Refusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s has exited: restart the agent first", req.ID)), nil
	}
	return p.RunSessionAction(req)
}

// actionRun follows the session's status on its own watch, not the saved
// meta, which can still lag behind the Idle a step just saw.
type actionRun struct {
	p      *Prompt
	dir    string
	holder *holder.HolderClient
	msgs   <-chan holder.WatchMessage
	state  string
	// background marks a Working state whose turn is over, with a background
	// task still running; Promptable treats it as Idle.
	background bool
	started    time.Duration
	// convID is the last conversation id seen; convChanged is set when a
	// status carried a different one, and cleared at each step.
	convID      string
	convChanged bool
	// What the step's start signal saw, cleared at each step: a running turn, a
	// Blocked state, or the signal that ended it on the spot.
	sawWorking, wentBlocked bool
	ended                   string
	// personTyped is set once a person's own key or paste reaches the session.
	personTyped bool
	// gate, when set, runs under the typing lock before typeStepWith types; an
	// error stops the step.
	gate func() error
	RunOpts
}

// SendAction sends each prompt once the one before it has finished its turn;
// a step that finds the session not Idle, or whose prompt never starts,
// drops the rest. A step that fails raises a gx Actionable named after label
// with the prompts it did not send, so the failure shows instead of only
// reaching the log.
func (p *Prompt) SendAction(dir, label string, action []string) {
	i, err := p.runAction(dir, action)
	switch {
	case err == nil:
	case errors.Is(err, ErrDraft):
		p.log.Info("draft would not clear", "session", filepath.Base(dir), "unsent", len(action)-i)
		raiseBlocking(p, dir, DraftReason, wire.BlockingDraft, action[i:])
	case errors.Is(err, ErrServerStopping), errors.Is(err, ErrWentBlocked), errors.Is(err, errNotIdle):
		// Shutdown is not a failure, and Blocked or a busy session already shows.
		p.log.Info(strings.ToLower(label)+" stopped", "session", filepath.Base(dir), "err", err)
	case errors.Is(err, deliver.ErrMaybeUnsent):
		// No Action: resending could run the command twice.
		p.log.Warn(strings.ToLower(label)+" undecided", "session", filepath.Base(dir), "err", err)
		a := deliveryCard(action[i], err)
		a.Reason = undecidedStepReason(a.Reason, len(action)-i-1)
		p.raiseActionable(dir, a)
	default:
		p.log.Warn(strings.ToLower(label)+" failed", "session", filepath.Base(dir), "err", err)
		p.RaiseGxActionable(dir, fmt.Sprintf("%s failed: %v", label, err), action[i:])
	}
}

var errNotIdle = errors.New("session is not idle")

// undecidedStepReason is the card of an undecided step: Deliver's reason, then
// how many later steps were dropped.
func undecidedStepReason(reason string, later int) string {
	switch later {
	case 0:
		return reason
	case 1:
		return reason + " 1 later step not sent."
	}
	return fmt.Sprintf("%s %d later steps not sent.", reason, later)
}

// runAction sends the prompts and returns the index of the step it stopped
// at, with the error it stopped on.
func (p *Prompt) runAction(dir string, action []string) (int, error) {
	run, closeRun, err := p.openActionRun(dir, RunOpts{})
	if err != nil {
		return 0, err
	}
	defer closeRun()
	for i, prompt := range action {
		if err := run.CatchUp(); err != nil {
			return i, err
		}
		if !run.Promptable() {
			return i, fmt.Errorf("step %d: %w (%s)", i, errNotIdle, run.State())
		}
		if err := run.Step(i, prompt); err != nil {
			return i, err
		}
	}
	return len(action), nil
}

// openActionRun dials the session's holder and starts following its watch;
// the returned func ends the watch.
func (p *Prompt) openActionRun(dir string, opts RunOpts) (*actionRun, func(), error) {
	c, err := holder.DialHolder(dir)
	if err != nil {
		return nil, nil, err
	}
	w, err := c.Watch()
	if err != nil {
		return nil, nil, err
	}
	done := make(chan struct{})
	msgs := make(chan holder.WatchMessage)
	go func() {
		defer close(msgs)
		for {
			msg, err := w.Next()
			if err != nil {
				return
			}
			select {
			case msgs <- msg:
			case <-done:
				return
			}
		}
	}()
	run := &actionRun{
		RunOpts: opts,
		p:       p, dir: dir, holder: c, msgs: msgs, state: wire.SessionIdle,
		started: cmp.Or(p.cfg.ActionStepStartTimeout, DefaultActionStepStartTimeout),
	}
	return run, func() {
		close(done)
		_ = w.Close()
	}, nil
}

func (r *actionRun) State() string                { return r.state }
func (r *actionRun) Holder() *holder.HolderClient { return r.holder }
func (r *actionRun) PersonTyped() bool            { return r.personTyped }
func (r *actionRun) ClearPersonTyped()            { r.personTyped = false }

// TypeStep types one prompt and presses its Enter, without waiting.
func (r *actionRun) TypeStep(i int, prompt string) error {
	return r.typeStepWith(i, prompt, imageSet{})
}

// typeStepWith is TypeStep that pastes imgs after the draft check and before
// the prompt. The lock, the draft clear, the paste, the keys and the nudges
// are deliver.Deliver's; the run's status watch is its start signal.
func (r *actionRun) typeStepWith(i int, prompt string, imgs imageSet) error {
	id := filepath.Base(r.dir)
	slash := strings.HasPrefix(prompt, "/")
	t := &deliver.Target{C: r.holder, Typing: r.p.projects.SessionTyping(r.dir), IO: r.p.projects.TypingIO(r.dir, r.holder, "action-step"),
		Cancel: r.Cancel, OnTyped: r.OnTyped,
		Lock:  func() (func(), error) { return lockTyping(r.p, id, r.Cancel) },
		Start: func(timeout time.Duration) (bool, error) { return r.waitStepStart(timeout, slash) }}
	imgs.onto(t)
	if r.gate != nil {
		// The gate runs under the lock; Deliver's first keys take it over.
		unlock, err := lockTyping(r.p, id, r.Cancel)
		if err != nil {
			return err
		}
		release, err := deliver.HoldInput(r.holder, unlock)
		if err != nil {
			return err
		}
		t.Held = release
		defer t.ReleaseHeld()
		if err := r.gate(); err != nil {
			return err
		}
	}
	r.convChanged, r.sawWorking, r.wentBlocked, r.ended = false, false, false, ""
	r.p.log.Info("action step typing", "session", id, "step", i, "len", len(prompt), "head", PromptHead(prompt))
	policy := r.p.deliverPolicy(r.dir, prompt, r.started, "action step nudge")
	if err := deliver.Deliver(t, prompt, policy); err != nil {
		return err
	}
	r.p.log.Info("action step typed", "session", id, "step", i)
	return nil
}

// waitStepStart is a step's start signal for Deliver. A running turn is a
// start; so is a Blocked state (the prompt was sent) and a person's own input.
// A slash step also starts on a new conversation id while Idle (`/clear`).
func (r *actionRun) waitStepStart(timeout time.Duration, slash bool) (bool, error) {
	timer := time.After(timeout)
	for {
		expired, signal, err := r.NextStatus(timer)
		switch {
		case errors.Is(err, ErrWentBlocked):
			r.wentBlocked = true
			return true, nil
		case err != nil:
			return false, err
		case signal != "":
			r.ended = signal
			return true, nil
		case r.TurnRunning():
			r.sawWorking = true
			return true, nil
		case slash && r.Promptable() && r.convChanged:
			r.ended = signalNewConv
			return true, nil
		case expired:
			return false, nil
		}
	}
}

// Step types one prompt and waits for it to end.
func (r *actionRun) Step(i int, prompt string) error {
	begin := time.Now()
	if err := r.TypeStep(i, prompt); err != nil {
		if errors.Is(err, claudeui.ErrStartTimeout) || errors.Is(err, claudeui.ErrStuckSubmission) {
			r.p.log.Info("action step done", "session", filepath.Base(r.dir), "step", i, "signal", signalStartTimeout, "elapsed", time.Since(begin))
			return fmt.Errorf("step %d: %w", i, err)
		}
		return err
	}
	signal, err := r.waitForStepEnd()
	r.p.log.Info("action step done", "session", filepath.Base(r.dir), "step", i, "signal", signal, "elapsed", time.Since(begin))
	if err != nil {
		return fmt.Errorf("step %d: %w", i, err)
	}
	return nil
}

// CatchUp applies the statuses already sent.
func (r *actionRun) CatchUp() error {
	for {
		select {
		case msg, ok := <-r.msgs:
			if err := r.apply(msg, ok); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

// Signals a step can end on, named in its debug line.
const (
	signalTurn         = "turn"
	signalNewConv      = "new-conversation"
	signalSettle       = "settle"
	signalStartTimeout = "start-timeout"
	signalPersonInput  = "person-input"
)

// NextStatus blocks for the next event of a step. It reports expired when the
// timer fires. Otherwise a non-empty signal or an error means the step is over,
// and neither means a status was applied to r.state. Blocked stops the rest
// rather than wait on a person who may never come.
func (r *actionRun) NextStatus(timer <-chan time.Time) (expired bool, signal string, err error) {
	select {
	case <-r.p.sessions.Watches.Stop:
		return false, "", ErrServerStopping
	case <-r.Cancel:
		return false, signalPersonInput, nil
	case <-timer:
		return true, "", nil
	case msg, ok := <-r.msgs:
		if err := r.apply(msg, ok); err != nil {
			return false, "", err
		}
		if r.state == wire.SessionBlocked {
			return false, "", ErrWentBlocked
		}
		return false, "", nil
	}
}

// waitForStepEnd waits for a typed step to end, after Deliver's start signal
// has noted how it began. A running turn ends on the following Idle (read as
// promptable: a finished turn that left a background task running stays
// Working and never turns Idle). A command that never showed a turn ends on
// Deliver's settle, which already pressed any Enter it needed.
func (r *actionRun) waitForStepEnd() (string, error) {
	if err := r.CatchUp(); err != nil {
		return "", err
	}
	switch {
	case r.ended != "":
		return r.ended, nil
	case r.wentBlocked:
		return "", ErrWentBlocked
	case r.sawWorking || r.TurnRunning():
	default:
		return signalSettle, nil
	}
	for !r.Promptable() {
		if _, signal, err := r.NextStatus(nil); signal != "" || err != nil {
			return signal, err
		}
	}
	return signalTurn, nil
}

// Promptable reports whether the session waits for the person, as
// SessionStatus.Promptable does.
func (r *actionRun) Promptable() bool {
	return wire.SessionStatus{State: r.state, Background: r.background}.Promptable()
}

// TurnRunning reports whether a turn is running: Working, but not a finished
// turn that only left a background task.
func (r *actionRun) TurnRunning() bool {
	return r.state == wire.SessionWorking && !r.background
}

func (r *actionRun) apply(msg holder.WatchMessage, ok bool) error {
	if !ok || msg.Type == holder.StreamExit {
		return errors.New("session ended")
	}
	if msg.Type == holder.StreamInput {
		r.personTyped = true
	}
	if msg.Type == holder.StreamStatus {
		st := sessions.SessionStatusOf(msg.Status)
		r.state, r.background = st.State, st.Background
		log := r.p.log.With("session", filepath.Base(r.dir))
		if id := msg.ConversationID; id != "" && id != r.convID {
			// The first id seen is the baseline, not a change.
			if r.convID != "" {
				r.convChanged = true
				log.Debug("action step conversation changed", "id", id)
			}
			r.convID = id
		}
	}
	return nil
}
