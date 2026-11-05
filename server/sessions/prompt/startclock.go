package prompt

import (
	"time"

	"github.com/elentok/gx/server/wire"
)

// StartClock counts only time spent Starting, so a dialog a person answers
// (Blocked) does not use it up. Only the watch loop touches it; the timer
// callback reads the session's state from its meta.
type StartClock struct {
	timeout time.Duration
	timer   *time.Timer
	since   time.Time
	spent   time.Duration
}

// NewStartClock starts the clock at launch; onTimeout fires when the
// session has spent timeout Starting.
func NewStartClock(timeout time.Duration, onTimeout func()) *StartClock {
	return &StartClock{
		timeout: timeout,
		timer:   time.AfterFunc(timeout, onTimeout),
		since:   time.Now(),
	}
}

// Track pauses the clock outside Starting and resumes it with what is left.
func (c *StartClock) Track(state string) {
	starting := state == wire.SessionStarting || state == ""
	switch {
	case starting && c.since.IsZero():
		c.since = time.Now()
		c.timer.Reset(c.timeout - c.spent)
	case !starting && !c.since.IsZero():
		c.spent += time.Since(c.since)
		c.since = time.Time{}
		c.timer.Stop()
	}
}

// Stop ends the clock.
func (c *StartClock) Stop() { c.timer.Stop() }
