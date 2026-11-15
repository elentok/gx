package skills

import (
	"sync"
	"sync/atomic"
)

// SharedRefresh runs a refresh so callers that overlap share one run and its
// result. Several launches can hit an unknown skill at once; one reinstall
// serves them all.
type SharedRefresh struct {
	run    func() error
	mu     sync.Mutex
	flight *refreshFlight
	// waiting counts callers that joined a run; tests poll it instead of sleeping.
	waiting atomic.Int32
}

type refreshFlight struct {
	done chan struct{}
	err  error
}

// NewSharedRefresh wraps run, usually a closure over Refresh.
func NewSharedRefresh(run func() error) *SharedRefresh {
	return &SharedRefresh{run: run}
}

// Refresh runs the wrapped refresh, or waits for the one already running.
func (s *SharedRefresh) Refresh() error {
	s.mu.Lock()
	if f := s.flight; f != nil {
		s.waiting.Add(1)
		s.mu.Unlock()
		<-f.done
		return f.err
	}
	f := &refreshFlight{done: make(chan struct{})}
	s.flight = f
	s.mu.Unlock()
	f.err = s.run()
	s.mu.Lock()
	s.flight = nil
	s.mu.Unlock()
	close(f.done)
	return f.err
}
