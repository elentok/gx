package ralphloop

import (
	"errors"
	"fmt"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
)

// holdRateLimit keeps a rate-limited agent in place: no nudge, no pause kind,
// no needs-answer. It sends "continue" once the reset (plus rateLimitResetBuffer)
// has passed, or every rateLimitPollInterval when the reset is unknown, and
// returns when a turn ends without the runner reporting a limit. It asks the
// runner after each turn, not the transcript alone: on PTY an idle agent's
// transcript keeps the old error as its newest entry.
func holdRateLimit(d Deps, o OneIteration, p launchAndPromptParams, waitStates []agentrunner.State, resetAt time.Time) error {
	sessionID := ""
	if status, err := d.Runner.Status(p.session()); err == nil {
		sessionID = status.SessionID
	}
	reason := "rate limit detected"
	if !resetAt.IsZero() {
		reason = fmt.Sprintf("rate limit detected, resets %s", resetAt.UTC().Format(time.RFC3339))
	}
	p.sink().IterationPaused(p.Ticket, p.Label, PauseRateLimit, reason)
	p.logAgentEvent(string(events.PausedRateLimit), sessionID, reason)
	for {
		if o.OnRateLimit != nil {
			o.OnRateLimit(true, resetAt)
		}
		if resetAt.IsZero() {
			d.Sleep(rateLimitPollInterval)
		} else if wait := resetAt.Add(rateLimitResetBuffer).Sub(d.Now()); wait > 0 {
			d.Sleep(wait)
		}
		// A refused prompt means the agent is already working (claude's own
		// auto-continue) or blocked; the wait below and the caller handle both.
		if err := d.Runner.Prompt(p.session(), "continue"); err != nil && !errors.Is(err, agentrunner.ErrNotReady) {
			return fmt.Errorf("re-prompting %s after rate-limit reset: %w", p.Label, err)
		}
		for {
			_, err := d.Runner.Wait(p.session(), waitStates, smartZonePollMs*time.Millisecond)
			if err == nil {
				break
			}
			if !errors.Is(err, agentrunner.ErrTimeout) {
				return err
			}
		}
		var limited bool
		resetAt, limited, _, _ = runnerRateLimit(d, p)
		if !limited {
			break
		}
	}
	if o.OnRateLimit != nil {
		o.OnRateLimit(false, time.Time{})
	}
	p.sink().IterationResumed(p.Ticket, p.Label, PauseRateLimit)
	p.logLifecycleEvent(string(events.Resumed), sessionID)
	return nil
}
