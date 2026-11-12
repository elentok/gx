package rigs

import (
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// SchedulesClock is a clock a test moves by hand.
type SchedulesClock struct {
	mu sync.Mutex
	t  time.Time
}

func NewSchedulesClock(t time.Time) *SchedulesClock { return &SchedulesClock{t: t} }

func (c *SchedulesClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *SchedulesClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}

// SchedulesInterval is the scheduler's check interval under test.
const SchedulesInterval = 20 * time.Millisecond

// StartSchedulesHarness is StartOneOffHarness with the scheduler on clock and a
// short check interval. The queue is paused, so started runs stay queued.
func StartSchedulesHarness(t *testing.T, clock *SchedulesClock) (*servertest.Harness, string) {
	t.Helper()
	return StartOneOffHarness(t, WithSchedulesClock(clock))
}

// WithSchedulesClock runs the scheduler on clock with a short check interval.
func WithSchedulesClock(clock *SchedulesClock) func(*server.Config) {
	return func(c *server.Config) {
		c.SchedulesClock = clock.Now
		c.SchedulesInterval = SchedulesInterval
	}
}
