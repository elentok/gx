package supervise

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/runlog"
)

// RateLimitPollInterval is how often HoldRateLimit re-checks a pane when the
// reset time in its rate-limit message couldn't be parsed.
const RateLimitPollInterval = 5 * time.Minute

// RateLimitResetBuffer is added past a successfully-parsed reset time so the
// loop wakes just after the quota actually rolls over, not right at it.
const RateLimitResetBuffer = 60 * time.Second

// RateLimitReleaseFileEnv names a file whose existence ends a rate-limit hold
// early. Only the e2e fixture sets it, so a flow can keep the window open as
// long as it needs instead of racing a real-time reset.
const RateLimitReleaseFileEnv = "GX_RATE_LIMIT_RELEASE_FILE"

const rateLimitReleasePoll = 100 * time.Millisecond

// sleepRateLimit waits out a hold, ending it early once the release file exists.
func sleepRateLimit(d deps.Deps, wait time.Duration) {
	file := os.Getenv(RateLimitReleaseFileEnv)
	if file == "" {
		d.Sleep(wait)
		return
	}
	for ; wait > 0; wait -= rateLimitReleasePoll {
		if _, err := os.Stat(file); err == nil {
			return
		}
		d.Sleep(min(wait, rateLimitReleasePoll))
	}
}

func codexBlocked(d deps.Deps, p LaunchParams) bool {
	st, err := d.Runner.Status(p.SessionOrDefault())
	return err == nil && st.State == agentrunner.StateBlocked
}

// HoldRateLimit keeps a rate-limited agent in place: no nudge, no pause kind,
// no question ticket. It sends "continue" once the reset (plus RateLimitResetBuffer)
// has passed, or every RateLimitPollInterval when the reset is unknown, and
// returns when a turn ends without the runner reporting a limit. It asks the
// runner after each turn, not the transcript alone: on PTY an idle agent's
// transcript keeps the old error as its newest entry.
func HoldRateLimit(d deps.Deps, onRateLimit func(limited bool, resetAt time.Time), p LaunchParams, waitStates []agentrunner.State, resetAt time.Time) error {
	sessionID := ""
	if status, err := d.Runner.Status(p.SessionOrDefault()); err == nil {
		sessionID = status.SessionID
	}
	reason := "rate limit detected"
	if !resetAt.IsZero() {
		reason = fmt.Sprintf("rate limit detected, resets %s", resetAt.UTC().Format(time.RFC3339))
	}
	p.EventSink().IterationPaused(p.Ticket, p.Label, runlog.PauseRateLimit, reason)
	if err := p.LogAgentEvent(string(events.PausedRateLimit), sessionID, reason); err != nil {
		return err
	}
	for {
		if onRateLimit != nil {
			onRateLimit(true, resetAt)
		}
		if resetAt.IsZero() {
			d.Sleep(RateLimitPollInterval)
		} else if wait := resetAt.Add(cmp.Or(d.RateLimitResetBuffer, RateLimitResetBuffer)).Sub(d.Now()); wait > 0 {
			sleepRateLimit(d, wait)
		}
		// A Codex pane back on its own dialog is not waiting for "continue": that
		// is a prompt, not a dialog answer. Release the hold and let the caller's
		// blocked rule pause it.
		if !(p.Agent == agent.AgentCodex && codexBlocked(d, p)) {
			// A refused prompt means the agent is already working (claude's own
			// auto-continue) or blocked; the wait below and the caller handle both.
			if err := d.Runner.Prompt(p.SessionOrDefault(), "continue"); err != nil && !errors.Is(err, agentrunner.ErrNotReady) {
				return fmt.Errorf("re-prompting %s after rate-limit reset: %w", p.Label, err)
			}
		}
		for {
			_, err := d.Runner.Wait(p.SessionOrDefault(), waitStates, SmartZonePollMs*time.Millisecond)
			if err == nil {
				break
			}
			if !errors.Is(err, agentrunner.ErrTimeout) {
				return err
			}
		}
		var limited bool
		resetAt, limited, _, _ = RunnerRateLimit(d, p)
		if !limited {
			break
		}
	}
	if onRateLimit != nil {
		onRateLimit(false, time.Time{})
	}
	p.EventSink().IterationResumed(p.Ticket, p.Label, runlog.PauseRateLimit)
	return p.LogLifecycleEvent(string(events.Resumed), sessionID)
}
