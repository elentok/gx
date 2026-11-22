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
	clock    Clock
	fire     func()

	mu      sync.Mutex
	last    time.Time
	stop    func() bool // non-nil while a trailing fire is pending
	stopped bool
}

// Clock is the throttle's time source. A test sets one to place triggers at
// chosen instants; the zero value is the wall clock.
type Clock struct {
	Now func() time.Time
	// AfterFunc runs f once after d and returns a function that cancels it.
	AfterFunc func(d time.Duration, f func()) (stop func() bool)
}

func (c Clock) orReal() Clock {
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.AfterFunc == nil {
		c.AfterFunc = func(d time.Duration, f func()) func() bool { return time.AfterFunc(d, f).Stop }
	}
	return c
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
	return NewThrottleClock(interval, Clock{}, fire)
}

// NewThrottleClock is NewThrottle on a chosen clock.
func NewThrottleClock(interval time.Duration, clock Clock, fire func()) *Throttle {
	return &Throttle{interval: interval, clock: clock.orReal(), fire: fire}
}

// Trigger records that something changed.
func (t *Throttle) Trigger() {
	t.mu.Lock()
	if t.stopped || t.stop != nil {
		t.mu.Unlock()
		return
	}
	now := t.clock.Now()
	wait := t.interval - now.Sub(t.last)
	if t.last.IsZero() || wait <= 0 {
		t.last = now
		t.mu.Unlock()
		t.fire()
		return
	}
	t.stop = t.clock.AfterFunc(wait, t.trailing)
	t.mu.Unlock()
}

func (t *Throttle) trailing() {
	t.mu.Lock()
	t.stop = nil
	if t.stopped {
		t.mu.Unlock()
		return
	}
	t.last = t.clock.Now()
	t.mu.Unlock()
	t.fire()
}

// Stop cancels a pending trailing fire and ignores later triggers.
func (t *Throttle) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopped = true
	if t.stop != nil {
		t.stop()
		t.stop = nil
	}
}
