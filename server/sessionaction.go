package server

import (
	"cmp"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/nativerunner"
)

// DefaultActionStepStartTimeout is how long an Action step waits for its
// prompt to go Working before the rest of the Action is dropped.
const DefaultActionStepStartTimeout = 10 * time.Second

// DefaultActionSlashSettle is how long a slash step stays Idle after typing
// before it counts as done: claude runs some commands locally and never
// goes Working.
const DefaultActionSlashSettle = time.Second

// runSessionAction clears an Idle session's Actionable and sends its Action
// in the background, so the run outlives the client that asked for it.
func (s *Server) runSessionAction(req SessionRequest) (SessionResult, error) {
	s.sessionWatches.metaMu.Lock()
	defer s.sessionWatches.metaMu.Unlock()
	dir, meta, res, err := s.sessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if st := meta.info().Status.State; st != SessionIdle {
		return sessionRefusal(ReasonSessionNotIdle, fmt.Sprintf("session %s is %s, not idle", req.ID, st)), nil
	}
	action := meta.Actionable.Action
	if len(action) == 0 {
		return sessionRefusal(ReasonSessionNoAction, fmt.Sprintf("session %s has no action", req.ID)), nil
	}
	meta.Actionable = SessionActionable{}
	if err := writeSessionMeta(dir, meta); err != nil {
		return SessionResult{}, err
	}
	s.events.publish(EventSessionChanged, req.ID)
	s.sessionWatches.start(func() {
		if err := s.sendAction(dir, action); err != nil {
			s.log.Info("action stopped", "session", req.ID, "err", err)
		}
	})
	info := meta.info()
	return SessionResult{Session: &info}, nil
}

// actionRun follows the session's status on its own watch, not the saved
// meta, which can still lag behind the Idle a step just saw.
type actionRun struct {
	s       *Server
	dir     string
	holder  *nativerunner.HolderClient
	msgs    <-chan nativerunner.WatchMessage
	state   string
	started time.Duration
	settle  time.Duration
	// convID is the last conversation id seen; convChanged is set when a
	// status carried a different one, and cleared at each step.
	convID      string
	convChanged bool
	// sawStatus is set by the first status of a step, for the debug line.
	sawStatus bool
	// onTyped runs once a step's prompt is typed. cancel, when closed, ends a
	// step's wait without another Enter: the first prompt sets both so a
	// person's own input stops it.
	onTyped func()
	cancel  <-chan struct{}
}

// sendAction sends each prompt once the one before it has finished its turn;
// a step that finds the session not Idle, or whose prompt never starts,
// drops the rest.
func (s *Server) sendAction(dir string, action []string) error {
	run, closeRun, err := s.openActionRun(dir)
	if err != nil {
		return err
	}
	defer closeRun()
	for i, prompt := range action {
		if err := run.catchUp(); err != nil {
			return err
		}
		if run.state != SessionIdle {
			return fmt.Errorf("step %d: session is %s", i, run.state)
		}
		if err := run.step(i, prompt); err != nil {
			if errors.Is(err, errDraft) {
				s.refuseForDraft(dir, action[i:])
				return nil
			}
			return err
		}
	}
	return nil
}

// refuseForDraft raises the Actionable that keeps the prompts gx did not type.
func (s *Server) refuseForDraft(dir string, unsent []string) {
	s.log.Info("refused to type over a draft", "session", filepath.Base(dir), "unsent", len(unsent))
	s.raiseGxActionable(dir, draftReason, unsent)
}

// openActionRun dials the session's holder and starts following its watch;
// the returned func ends the watch.
func (s *Server) openActionRun(dir string) (*actionRun, func(), error) {
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
		s: s, dir: dir, holder: c, msgs: msgs, state: SessionIdle,
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
	if !sessionTyping.ReadyToType(nativerunner.HolderTypingIO(r.holder)) {
		return fmt.Errorf("step %d: not ready to type", i)
	}
	if r.s.inputHoldsDraft(r.dir, r.holder) {
		return errDraft
	}
	r.convChanged, r.sawStatus = false, false
	begin := time.Now()
	if err := typeSessionPrompt(r.dir, prompt); err != nil {
		return err
	}
	if r.onTyped != nil {
		r.onTyped()
	}
	r.s.log.Debug("action step typed", "session", filepath.Base(r.dir), "step", i)
	signal, err := r.waitForStep(strings.HasPrefix(prompt, "/"))
	r.s.log.Debug("action step done", "session", filepath.Base(r.dir), "step", i, "signal", signal, "elapsed", time.Since(begin))
	if err != nil {
		return fmt.Errorf("step %d: %w", i, err)
	}
	return nil
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

// waitForStep waits for a step to end and returns the signal it ended on. A
// real prompt ends on Working then Idle, and stops the rest if it never goes
// Working. A slash command may run locally in claude and never show Working,
// so it also ends on a new conversation id while Idle (`/clear`), or on the
// settle window with the session still Idle, where a swallowed Enter gets
// one more. Blocked stops the rest rather than wait on a person who may never
// come.
func (r *actionRun) waitForStep(slash bool) (string, error) {
	timer := r.started
	if slash {
		timer = r.settle
	}
	expired := time.After(timer)
	working := false
	for {
		select {
		case <-r.s.sessionWatches.stop:
			return "", errors.New("server stopping")
		case <-r.cancel:
			return signalPersonInput, nil
		case <-expired:
			if working {
				continue
			}
			if !slash {
				return signalStartTimeout, errors.New("prompt never went working")
			}
			if r.state != SessionIdle {
				continue
			}
			pressed, err := pressEnterIfStillTyped(r.dir)
			if pressed {
				r.s.log.Debug("action step settle enter", "session", filepath.Base(r.dir))
			}
			return signalSettle, err
		case msg, ok := <-r.msgs:
			if err := r.apply(msg, ok); err != nil {
				return "", err
			}
			switch {
			case r.state == SessionBlocked:
				return "", errors.New("session went blocked")
			case r.state == SessionWorking:
				working = true
			case r.state == SessionIdle && working:
				return signalTurn, nil
			case r.state == SessionIdle && slash && r.convChanged:
				return signalNewConv, nil
			}
		}
	}
}

func (r *actionRun) apply(msg nativerunner.WatchMessage, ok bool) error {
	if !ok || msg.Type == nativerunner.StreamExit {
		return errors.New("session ended")
	}
	if msg.Type == nativerunner.StreamStatus {
		r.state = sessionStatusOf(msg.Status).State
		log := r.s.log.With("session", filepath.Base(r.dir))
		if !r.sawStatus {
			r.sawStatus = true
			log.Debug("action step first status", "state", r.state)
		}
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
