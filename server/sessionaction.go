package server

import (
	"cmp"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/nativerunner"
)

// runSessionAction clears an Idle session's Actionable and sends its Action
// in the background, so the run outlives the client that asked for it.
func (s *Server) runSessionAction(req SessionRequest) (SessionResult, error) {
	return s.sendSessionAction(req, false)
}

// sendSessionAction is runSessionAction; auto marks a send the server made
// itself, which the runaway guard counts. Any other send is the person's input
// and resets that count.
func (s *Server) sendSessionAction(req SessionRequest, auto bool) (SessionResult, error) {
	s.sessionWatches.metaMu.Lock()
	defer s.sessionWatches.metaMu.Unlock()
	dir, meta, res, err := s.sessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if st := meta.info().Status; !st.Promptable() {
		return sessionRefusal(ReasonSessionNotIdle, fmt.Sprintf("session %s is %s, not idle", req.ID, st.State)), nil
	}
	action := meta.Actionable.Action
	if len(action) == 0 {
		return sessionRefusal(ReasonSessionNoAction, fmt.Sprintf("session %s has no action", req.ID)), nil
	}
	meta.Actionable = SessionActionable{}
	if auto {
		meta.AutoSends++
	} else {
		s.cancelAutoAdvance(req.ID, "Auto-advance stopped: you ran the action")
		meta.AutoSends = 0
	}
	if err := writeSessionMeta(dir, meta); err != nil {
		return SessionResult{}, err
	}
	s.events.publish(EventSessionChanged, req.ID)
	s.startContinue(dir, meta, "Action", action)
	info := meta.info()
	return SessionResult{Session: &info}, nil
}

// actionRun follows the session's status on its own watch, not the saved
// meta, which can still lag behind the Idle a step just saw.
type actionRun struct {
	s      *Server
	dir    string
	holder *nativerunner.HolderClient
	msgs   <-chan nativerunner.WatchMessage
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
	actionRunOpts
}

// actionRunOpts hooks a run into a person's own input. onTyped runs once a
// step's prompt is typed. cancel, when closed, ends a step's wait without
// another Enter: the first prompt sets both so a person's own input stops it.
type actionRunOpts struct {
	onTyped func()
	cancel  <-chan struct{}
}

// sendAction sends each prompt once the one before it has finished its turn;
// a step that finds the session not Idle, or whose prompt never starts,
// drops the rest. A step that fails raises a gx Actionable named after label
// with the prompts it did not send, so the failure shows instead of only
// reaching the log.
func (s *Server) sendAction(dir, label string, action []string) {
	i, err := s.runAction(dir, action)
	switch {
	case err == nil:
	case errors.Is(err, errDraft):
		s.log.Info("draft would not clear", "session", filepath.Base(dir), "unsent", len(action)-i)
		s.raiseGxActionable(dir, draftReason, action[i:])
	case errors.Is(err, errServerStopping), errors.Is(err, errWentBlocked), errors.Is(err, errNotIdle):
		// Shutdown is not a failure, and Blocked or a busy session already shows.
		s.log.Info(strings.ToLower(label)+" stopped", "session", filepath.Base(dir), "err", err)
	default:
		s.log.Warn(strings.ToLower(label)+" failed", "session", filepath.Base(dir), "err", err)
		s.raiseGxActionable(dir, fmt.Sprintf("%s failed: %v", label, err), action[i:])
	}
}

// Errors that end an Action without raising an Actionable.
var (
	errServerStopping = errors.New("server stopping")
	errWentBlocked    = errors.New("session went blocked")
	errNotIdle        = errors.New("session is not idle")
)

// runAction sends the prompts and returns the index of the step it stopped
// at, with the error it stopped on.
func (s *Server) runAction(dir string, action []string) (int, error) {
	run, closeRun, err := s.openActionRun(dir, actionRunOpts{})
	if err != nil {
		return 0, err
	}
	defer closeRun()
	for i, prompt := range action {
		if err := run.catchUp(); err != nil {
			return i, err
		}
		if !run.promptable() {
			return i, fmt.Errorf("step %d: %w (%s)", i, errNotIdle, run.state)
		}
		if err := run.step(i, prompt); err != nil {
			return i, err
		}
	}
	return len(action), nil
}

// openActionRun dials the session's holder and starts following its watch;
// the returned func ends the watch.
func (s *Server) openActionRun(dir string, opts actionRunOpts) (*actionRun, func(), error) {
	c, err := nativerunner.DialHolder(dir)
	if err != nil {
		return nil, nil, err
	}
	w, err := c.Watch()
	if err != nil {
		return nil, nil, err
	}
	done := make(chan struct{})
	msgs := make(chan nativerunner.WatchMessage)
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
		actionRunOpts: opts,
		s:             s, dir: dir, holder: c, msgs: msgs, state: SessionIdle,
		started: cmp.Or(s.cfg.ActionStepStartTimeout, DefaultActionStepStartTimeout),
		settle:  cmp.Or(s.cfg.ActionSlashSettle, DefaultActionSlashSettle),
	}
	return run, func() {
		close(done)
		_ = w.Close()
	}, nil
}

// step types one prompt and waits for it to end.
func (r *actionRun) step(i int, prompt string) error {
	if !r.s.typing().ReadyToType(nativerunner.HolderTypingIO(r.holder)) {
		return fmt.Errorf("step %d: %w", i, claudeui.ErrNotReadyToType)
	}
	if err := r.s.clearDraft(r.dir, r.holder); err != nil {
		return err
	}
	r.convChanged = false
	begin := time.Now()
	before, _ := inputState(r.holder)
	r.s.log.Info("action step typing", "session", filepath.Base(r.dir), "step", i, "len", len(prompt), "head", promptHead(prompt), "input_before", before)
	if err := typeSessionPrompt(r.dir, prompt); err != nil {
		return err
	}
	if r.onTyped != nil {
		r.onTyped()
	}
	r.s.log.Info("action step typed", "session", filepath.Base(r.dir), "step", i)
	signal, err := r.waitForStep(strings.HasPrefix(prompt, "/"))
	r.s.log.Info("action step done", "session", filepath.Base(r.dir), "step", i, "signal", signal, "elapsed", time.Since(begin))
	if err != nil {
		return fmt.Errorf("step %d: %w", i, err)
	}
	return nil
}

// promptHead is the start of a prompt for a log line.
func promptHead(prompt string) string {
	if r := []rune(prompt); len(r) > 40 {
		return string(r[:40]) + "…"
	}
	return prompt
}

// catchUp applies the statuses already sent.
func (r *actionRun) catchUp() error {
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

// nextStatus blocks for the next event of a step. It reports expired when the
// timer fires. Otherwise a non-empty signal or an error means the step is over,
// and neither means a status was applied to r.state. Blocked stops the rest
// rather than wait on a person who may never come.
func (r *actionRun) nextStatus(timer <-chan time.Time) (expired bool, signal string, err error) {
	select {
	case <-r.s.sessionWatches.stop:
		return false, "", errServerStopping
	case <-r.cancel:
		return false, signalPersonInput, nil
	case <-timer:
		return true, "", nil
	case msg, ok := <-r.msgs:
		if err := r.apply(msg, ok); err != nil {
			return false, "", err
		}
		if r.state == SessionBlocked {
			return false, "", errWentBlocked
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
		expired, signal, err := r.nextStatus(timer)
		if signal != "" || err != nil {
			return signal, err
		}
		switch {
		case expired && !working:
			return signalStartTimeout, errors.New("prompt never went working")
		case expired:
		case r.turnRunning():
			working = true
		case r.promptable() && working:
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
		expired, signal, err := r.nextStatus(timer)
		if signal != "" || err != nil {
			return signal, err
		}
		switch {
		case expired && (working || !r.promptable()):
		case expired:
			pressed, err := r.pressEnterIfStillTyped()
			if pressed {
				r.s.log.Debug("action step settle enter", "session", filepath.Base(r.dir))
			}
			return signalSettle, err
		case r.turnRunning():
			working = true
		case r.promptable() && working:
			return signalTurn, nil
		case r.promptable() && r.convChanged:
			return signalNewConv, nil
		}
	}
}

// promptable reports whether the session waits for the person, as
// SessionStatus.Promptable does.
func (r *actionRun) promptable() bool {
	return SessionStatus{State: r.state, Background: r.background}.Promptable()
}

// turnRunning reports whether a turn is running: Working, but not a finished
// turn that only left a background task.
func (r *actionRun) turnRunning() bool {
	return r.state == SessionWorking && !r.background
}

func (r *actionRun) apply(msg nativerunner.WatchMessage, ok bool) error {
	if !ok || msg.Type == nativerunner.StreamExit {
		return errors.New("session ended")
	}
	if msg.Type == nativerunner.StreamInput {
		r.personTyped = true
	}
	if msg.Type == nativerunner.StreamStatus {
		st := sessionStatusOf(msg.Status)
		r.state, r.background = st.State, st.Background
		log := r.s.log.With("session", filepath.Base(r.dir))
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
