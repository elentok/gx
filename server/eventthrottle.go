package server

import (
	"sync"
	"time"
)

// DefaultLimitsEventInterval is the coalescing window for limits-changed when
// Config.LimitsEventInterval is zero.
const DefaultLimitsEventInterval = 2 * time.Second

// eventThrottle coalesces bursts: the first trigger fires at once, triggers
// inside the interval set one trailing fire for when the interval ends. Dropping
// them instead would leave an open menu on an old value. It owns no goroutine;
// only a pending trailing timer, which stop cancels.
type eventThrottle struct {
	interval time.Duration
	fire     func()

	mu      sync.Mutex
	last    time.Time
	timer   *time.Timer // non-nil while a trailing fire is pending
	stopped bool
}

// limitsEventInterval is the limits-changed window, defaulted when unset.
func limitsEventInterval(cfg Config) time.Duration {
	if cfg.LimitsEventInterval <= 0 {
		return DefaultLimitsEventInterval
	}
	return cfg.LimitsEventInterval
}

func newEventThrottle(interval time.Duration, fire func()) *eventThrottle {
	return &eventThrottle{interval: interval, fire: fire}
}

// trigger records that something changed.
func (t *eventThrottle) trigger() {
	t.mu.Lock()
	if t.stopped || t.timer != nil {
		t.mu.Unlock()
		return
	}
	wait := t.interval - time.Since(t.last)
	if t.last.IsZero() || wait <= 0 {
		t.last = time.Now()
		t.mu.Unlock()
		t.fire()
		return
	}
	t.timer = time.AfterFunc(wait, t.trailing)
	t.mu.Unlock()
}

func (t *eventThrottle) trailing() {
	t.mu.Lock()
	t.timer = nil
	if t.stopped {
		t.mu.Unlock()
		return
	}
	t.last = time.Now()
	t.mu.Unlock()
	t.fire()
}

// stop cancels a pending trailing fire and ignores later triggers.
func (t *eventThrottle) stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopped = true
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
}
