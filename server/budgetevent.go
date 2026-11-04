package server

import (
	"math"
	"sync"
	"time"

	"github.com/elentok/gx/server/broker"
)

// DefaultBudgetEventInterval is the throttle window of budget-changed when
// Config.BudgetEventInterval is zero.
const DefaultBudgetEventInterval = 60 * time.Second

// budgetEvents publishes budget-changed when today's total moves by a cent or
// more since the last publish. All three spend sums include today, so today's
// total is enough. The first reading only sets the baseline.
type budgetEvents struct {
	throttle *broker.Throttle

	mu        sync.Mutex
	seeded    bool
	current   int64 // latest reading, in cents
	published int64 // reading at the last publish (or the baseline)
}

func newBudgetEvents(interval time.Duration, publish func()) *budgetEvents {
	if interval <= 0 {
		interval = DefaultBudgetEventInterval
	}
	b := &budgetEvents{}
	b.throttle = broker.NewThrottle(interval, func() {
		b.mu.Lock()
		b.published = b.current
		b.mu.Unlock()
		publish()
	})
	return b
}

// observe records today's total after a poll.
func (b *budgetEvents) observe(total float64) {
	cents := int64(math.Round(total * 100))
	b.mu.Lock()
	b.current = cents
	if !b.seeded {
		b.seeded = true
		b.published = cents
		b.mu.Unlock()
		return
	}
	changed := cents != b.published
	b.mu.Unlock()
	if changed {
		b.throttle.Trigger()
	}
}

func (b *budgetEvents) stop() { b.throttle.Stop() }
