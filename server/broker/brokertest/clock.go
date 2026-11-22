// Package brokertest holds test doubles for the broker package.
package brokertest

import (
	"sync"
	"time"

	"github.com/elentok/gx/server/broker"
)

// FakeClock is a broker.Clock that only moves when Advance is called.
type FakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

type fakeTimer struct {
	at      time.Time
	f       func()
	stopped bool
}

func NewFakeClock() *FakeClock {
	return &FakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

// Clock returns the broker.Clock view of c.
func (c *FakeClock) Clock() broker.Clock {
	return broker.Clock{Now: c.Now, AfterFunc: c.afterFunc}
}

func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *FakeClock) afterFunc(d time.Duration, f func()) func() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{at: c.now.Add(d), f: f}
	c.timers = append(c.timers, t)
	return func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		was := !t.stopped
		t.stopped = true
		return was
	}
}

// Advance moves the clock by d and runs, on the calling goroutine, every timer
// that is now due.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	var due []*fakeTimer
	keep := c.timers[:0]
	for _, t := range c.timers {
		switch {
		case t.stopped:
		case !t.at.After(c.now):
			t.stopped = true
			due = append(due, t)
		default:
			keep = append(keep, t)
		}
	}
	c.timers = keep
	c.mu.Unlock()
	for _, t := range due {
		t.f()
	}
}
