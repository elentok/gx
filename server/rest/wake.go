package rest

import (
	"time"

	"github.com/elentok/gx/config"
)

// NextWake is the next instant at which the state changes with no event: the
// end of the running period, the start of the Bedtime warning and the start of
// Bedtime. A Break starts on an active minute, so it has no wake of its own.
// The second result is false when nothing is due.
func NextWake(cfg config.MentalHealthConfig, s State, now time.Time) (time.Time, bool) {
	var at time.Time
	pick := func(t time.Time) {
		if t.After(now) && (at.IsZero() || t.Before(at)) {
			at = t
		}
	}
	if cfg.Enabled && s.Period != nil {
		pick(s.Period.End)
	}
	e := &engine{s: s, cfg: cfg, now: now}
	if bed, ok := e.nextBedtimeStart(); ok {
		pick(bed.Add(-bedtimeWarnLead))
		pick(bed)
	}
	return at, !at.IsZero()
}
