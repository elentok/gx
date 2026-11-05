// Package supervise holds the types and guards that watch one launched agent:
// the launch params, the pause gate, and the per-launch checks.
package supervise

import (
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop/deps"
)

// PersonInput reports keys a person pressed in the agent's pane. A nil
// *PersonInput (a runner that cannot report them) never sees any.
type PersonInput struct {
	events <-chan struct{}
	stop   func()
}

// WatchPersonInput subscribes to s's person input, or returns nil when the
// runner does not report it or the subscription fails.
func WatchPersonInput(r agentrunner.Runner, s agentrunner.Session) *PersonInput {
	pi, ok := r.(agentrunner.PersonInputter)
	if !ok {
		return nil
	}
	events, stop, err := pi.PersonInput(s)
	if err != nil {
		return nil
	}
	return &PersonInput{events: events, stop: stop}
}

func (in *PersonInput) Close() {
	if in != nil {
		in.stop()
	}
}

// Typed reports whether a key arrived since the last call.
func (in *PersonInput) Typed() bool {
	if in == nil {
		return false
	}
	seen := false
	for {
		select {
		case <-in.events:
			seen = true
		default:
			return seen
		}
	}
}

// Sleep waits dur, then waits it again for every key that arrived meanwhile,
// so the wait ends a full dur after the last key.
func (in *PersonInput) Sleep(d deps.Deps, dur time.Duration) {
	d.Sleep(dur)
	for in.Typed() {
		d.Sleep(dur)
	}
}
