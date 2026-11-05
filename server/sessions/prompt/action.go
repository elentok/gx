package prompt

import (
	"cmp"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
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
	p.resumer.StartContinue(dir, meta, "Action", action)
	info := meta.Info()
	return wire.SessionResult{Session: &info}, nil
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
	settle     time.Duration
	// convID is the last conversation id seen; convChanged is set when a
	// status carried a different one, and cleared at each step.
	convID      string
	convChanged bool
	// personTyped is set once a person's own key or paste reaches the session.
	personTyped bool
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
		p.RaiseGxActionable(dir, DraftReason, action[i:])
	case errors.Is(err, ErrServerStopping), errors.Is(err, ErrWentBlocked), errors.Is(err, errNotIdle):
		// Shutdown is not a failure, and Blocked or a busy session already shows.
		p.log.Info(strings.ToLower(label)+" stopped", "session", filepath.Base(dir), "err", err)
	default:
		p.log.Warn(strings.ToLower(label)+" failed", "session", filepath.Base(dir), "err", err)
		p.RaiseGxActionable(dir, fmt.Sprintf("%s failed: %v", label, err), action[i:])
	}
}

var errNotIdle = errors.New("session is not idle")

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
		settle:  cmp.Or(p.cfg.ActionSlashSettle, DefaultActionSlashSettle),
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
	if !r.p.projects.Typing().ReadyToType(r.p.projects.TypingIO(r.dir, r.holder, "action-step")) {
		return fmt.Errorf("step %d: %w", i, claudeui.ErrNotReadyToType)
	}
	if err := r.p.ClearDraft(r.dir, r.holder); err != nil {
		return err
	}
	r.convChanged = false
	before, _ := InputState(r.holder)
	r.p.log.Info("action step typing", "session", filepath.Base(r.dir), "step", i, "len", len(prompt), "head", PromptHead(prompt), "input_before", before)
	if err := sessions.TypeSessionPrompt(r.dir, prompt); err != nil {
		return err
	}
	if r.OnTyped != nil {
		r.OnTyped()
	}
	r.p.log.Info("action step typed", "session", filepath.Base(r.dir), "step", i)
	return nil
}

// Step types one prompt and waits for it to end.
func (r *actionRun) Step(i int, prompt string) error {
	begin := time.Now()
	if err := r.TypeStep(i, prompt); err != nil {
		return err
	}
	signal, err := r.waitForStep(strings.HasPrefix(prompt, "/"))
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

// waitForStep waits for a step to end and returns the signal it ended on.
func (r *actionRun) waitForStep(slash bool) (string, error) {
	if slash {
		return r.waitForSlash()
	}
	return r.waitForTurn()
}

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

// The waits below read Idle as promptable: a finished turn that left a
// background task running stays Working and never turns Idle.

// waitForTurn waits for a real prompt to go Working then Idle. It stops the
// rest if the session never goes Working within the start timeout.
func (r *actionRun) waitForTurn() (string, error) {
	timer := time.After(r.started)
	working := false
	for {
		expired, signal, err := r.NextStatus(timer)
		if signal != "" || err != nil {
			return signal, err
		}
		switch {
		case expired && !working:
			return signalStartTimeout, errors.New("prompt never went working")
		case expired:
		case r.TurnRunning():
			working = true
		case r.Promptable() && working:
			return signalTurn, nil
		}
	}
}

// waitForSlash waits for a slash command to end. It may run locally in claude
// and never show Working, so it ends on Working then Idle, on a new
// conversation id while Idle (`/clear`), or on the settle window with the
// session still Idle, where a swallowed Enter gets one more.
func (r *actionRun) waitForSlash() (string, error) {
	timer := time.After(r.settle)
	working := false
	for {
		expired, signal, err := r.NextStatus(timer)
		if signal != "" || err != nil {
			return signal, err
		}
		switch {
		case expired && (working || !r.Promptable()):
		case expired:
			pressed, err := r.p.PressEnterIfStillTyped(r.dir, r.holder)
			if pressed {
				r.p.log.Debug("action step settle enter", "session", filepath.Base(r.dir))
			}
			return signalSettle, err
		case r.TurnRunning():
			working = true
		case r.Promptable() && working:
			return signalTurn, nil
		case r.Promptable() && r.convChanged:
			return signalNewConv, nil
		}
	}
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
