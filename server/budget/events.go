package budget

import (
	"math"
	"sync"
	"time"

	"github.com/elentok/gx/server/broker"
)

// DefaultEventInterval is the throttle window of budget-changed when
// Config.EventInterval is zero.
const DefaultEventInterval = 60 * time.Second

// eventFeed publishes budget-changed when today's total moves by a cent or
// more since the last publish. All three spend sums include today, so today's
// total is enough. The first reading only sets the baseline.
type eventFeed struct {
	throttle *broker.Throttle

	mu        sync.Mutex
	seeded    bool
	current   int64 // latest reading, in cents
	published int64 // reading at the last publish (or the baseline)
}

func newEventFeed(interval time.Duration, publish func()) *eventFeed {
	if interval <= 0 {
		interval = DefaultEventInterval
	}
	f := &eventFeed{}
	f.throttle = broker.NewThrottle(interval, func() {
		f.mu.Lock()
		f.published = f.current
		f.mu.Unlock()
		publish()
	})
	return f
}

// observe records today's total after a poll.
func (f *eventFeed) observe(total float64) {
	cents := int64(math.Round(total * 100))
	f.mu.Lock()
	f.current = cents
	if !f.seeded {
		f.seeded = true
		f.published = cents
		f.mu.Unlock()
		return
	}
	changed := cents != f.published
	f.mu.Unlock()
	if changed {
		f.throttle.Trigger()
	}
}

func (f *eventFeed) stop() { f.throttle.Stop() }
