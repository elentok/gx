package broker

import (
	"sync"
	"time"
)

// DefaultLimitsEventInterval is the coalescing window for limits-changed when
// Config.LimitsEventInterval is zero.
const DefaultLimitsEventInterval = 2 * time.Second

// Throttle coalesces bursts: the first trigger fires at once, triggers
// inside the interval set one trailing fire for when the interval ends. Dropping
// them instead would leave an open menu on an old value. It owns no goroutine;
// only a pending trailing timer, which Stop cancels.
type Throttle struct {
	interval time.Duration
	fire     func()

	mu      sync.Mutex
	last    time.Time
	timer   *time.Timer // non-nil while a trailing fire is pending
	stopped bool
}

// LimitsInterval is the limits-changed window for a configured interval,
// defaulted when unset.
func LimitsInterval(configured time.Duration) time.Duration {
	if configured <= 0 {
		return DefaultLimitsEventInterval
	}
	return configured
}

func NewThrottle(interval time.Duration, fire func()) *Throttle {
	return &Throttle{interval: interval, fire: fire}
}

// Trigger records that something changed.
func (t *Throttle) Trigger() {
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

func (t *Throttle) trailing() {
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

// Stop cancels a pending trailing fire and ignores later triggers.
func (t *Throttle) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopped = true
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
}
