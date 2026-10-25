package server

import (
	"cmp"
	"sync"
	"time"
)

// DefaultAutoAdvanceDelay is how long a run-end Continue waits before the
// server sends it.
const DefaultAutoAdvanceDelay = 10 * time.Second

// autoAdvances holds the running countdowns, one per session. They live in
// memory only: a restart drops them and leaves the Continue raised.
type autoAdvances struct {
	mu sync.Mutex
	by map[string]*autoAdvance
}

type autoAdvance struct {
	deadline time.Time
	cancel   chan struct{}
}

// deadline is when the session's countdown ends; zero with none running.
func (a *autoAdvances) deadline(id string) time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	if c := a.by[id]; c != nil {
		return c.deadline
	}
	return time.Time{}
}

// stop ends the session's countdown and reports whether one was running. It is
// the one place a countdown is cancelled from.
func (a *autoAdvances) stop(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	c := a.by[id]
	if c == nil {
		return false
	}
	close(c.cancel)
	delete(a.by, id)
	return true
}

// begin registers a countdown ending at deadline, replacing any running one.
func (a *autoAdvances) begin(id string, deadline time.Time) *autoAdvance {
	a.mu.Lock()
	defer a.mu.Unlock()
	if old := a.by[id]; old != nil {
		close(old.cancel)
	}
	if a.by == nil {
		a.by = map[string]*autoAdvance{}
	}
	c := &autoAdvance{deadline: deadline, cancel: make(chan struct{})}
	a.by[id] = c
	return c
}

// finish drops c unless a newer countdown replaced it, and reports whether c
// still stood.
func (a *autoAdvances) finish(id string, c *autoAdvance) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.by[id] != c {
		return false
	}
	delete(a.by, id)
	return true
}

// startAutoAdvance counts down, then sends the session's Continue action. It
// reports false, starting nothing, when the countdown is off in the server
// config.
func (s *Server) startAutoAdvance(id string) bool {
	delay := cmp.Or(s.cfg.AutoAdvanceDelay, DefaultAutoAdvanceDelay)
	if delay < 0 {
		return false
	}
	c := s.autoAdvances.begin(id, time.Now().Add(delay))
	s.sessionWatches.start(func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-c.cancel:
			return
		case <-s.sessionWatches.stop:
			return
		}
		if !s.autoAdvances.finish(id, c) {
			return
		}
		s.events.publish(EventSessionChanged, id)
		s.sendAutoAdvance(id)
	})
	return true
}

// sendAutoAdvance sends the Continue action if the session still waits on it
// with the switch on.
func (s *Server) sendAutoAdvance(id string) {
	_, meta, res, err := s.sessionDir(id)
	if res.Refused || err != nil || meta.AutoAdvanceOff || !meta.Actionable.Derived || meta.Actionable.Grilling {
		return
	}
	if res, err := s.runSessionAction(SessionRequest{ID: id}); err != nil || res.Refused {
		s.log.Info("auto-advance not sent", "session", id, "err", err, "reason", res.Reason)
	}
}
