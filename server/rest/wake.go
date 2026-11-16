package rest

import (
	"time"

	"github.com/elentok/gx/config"
)

// NextWake is the next instant at which the state changes with no event: the
// end of the running period, the start of the Bedtime warning and the start of
// Bedtime, plus the end of an idle gap that resets the Break count. A Break
// starts on an active minute, so it has no wake of its own.
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
	if cfg.Enabled && cfg.Break.Enabled && s.WorkCount > 0 && !s.LastActive.IsZero() {
		pick(s.LastActive.Add(time.Duration(cfg.Break.BreakMinutes) * time.Minute))
	}
	if bed, ok := nextBedtimeStart(cfg, s, now); ok {
		pick(bed.Add(-bedtimeWarnLead))
		pick(bed)
	}
	return at, !at.IsZero()
}
