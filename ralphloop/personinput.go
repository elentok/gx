package ralphloop

import (
	"time"

	"github.com/elentok/gx/agentrunner"
)

// personInput reports keys a person pressed in the agent's pane. A nil
// *personInput (a runner that cannot report them) never sees any.
type personInput struct {
	events <-chan struct{}
	stop   func()
}

// watchPersonInput subscribes to s's person input, or returns nil when the
// runner does not report it or the subscription fails.
func watchPersonInput(r agentrunner.Runner, s agentrunner.Session) *personInput {
	pi, ok := r.(agentrunner.PersonInputter)
	if !ok {
		return nil
	}
	events, stop, err := pi.PersonInput(s)
	if err != nil {
		return nil
	}
	return &personInput{events: events, stop: stop}
}

func (in *personInput) close() {
	if in != nil {
		in.stop()
	}
}

// typed reports whether a key arrived since the last call.
func (in *personInput) typed() bool {
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

// sleep waits dur, then waits it again for every key that arrived meanwhile,
// so the wait ends a full dur after the last key.
func (in *personInput) sleep(d Deps, dur time.Duration) {
	d.Sleep(dur)
	for in.typed() {
		d.Sleep(dur)
	}
}
