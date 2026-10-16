package server

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/elentok/gx/nativerunner"
)

// DefaultActionStepStartTimeout is how long an Action step waits for its
// prompt to go Working before the rest of the Action is dropped.
const DefaultActionStepStartTimeout = 10 * time.Second

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
	msgs    <-chan nativerunner.WatchMessage
	state   string
	started time.Duration
}

// sendAction sends each prompt once the one before it has finished its turn;
// a step that finds the session not Idle, or whose prompt never starts,
// drops the rest.
func (s *Server) sendAction(dir string, action []string) error {
	c, err := nativerunner.DialHolder(dir)
	if err != nil {
		return err
	}
	w, err := c.Watch()
	if err != nil {
		return err
	}
	done := make(chan struct{})
	defer w.Close()
	defer close(done)
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
	run := &actionRun{s: s, msgs: msgs, state: SessionIdle, started: s.cfg.ActionStepStartTimeout}
	if run.started <= 0 {
		run.started = DefaultActionStepStartTimeout
	}
	for i, prompt := range action {
		if err := run.catchUp(); err != nil {
			return err
		}
		if run.state != SessionIdle {
			return fmt.Errorf("step %d: session is %s", i, run.state)
		}
		if err := typeSessionPrompt(dir, prompt); err != nil {
			return err
		}
		if err := run.waitForTurn(strings.HasPrefix(prompt, "/")); err != nil {
			return fmt.Errorf("step %d: %w", i, err)
		}
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

// waitForTurn waits for the session to go Working, then back to Idle. A slash
// command may run locally in claude (`/clear`) and never show Working, so one
// still Idle when the start timeout fires counts as finished; a real prompt
// that never starts stops the rest. Blocked stops the rest rather than wait on
// a person who may never come.
func (r *actionRun) waitForTurn(slash bool) error {
	started := time.After(r.started)
	working := false
	for {
		select {
		case <-r.s.sessionWatches.stop:
			return errors.New("server stopping")
		case <-started:
			if working {
				continue
			}
			if slash && r.state == SessionIdle {
				return nil
			}
			return errors.New("prompt never went working")
		case msg, ok := <-r.msgs:
			if err := r.apply(msg, ok); err != nil {
				return err
			}
			switch {
			case r.state == SessionBlocked:
				return errors.New("session went blocked")
			case r.state == SessionWorking:
				working = true
			case r.state == SessionIdle && working:
				return nil
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
	}
	return nil
}
