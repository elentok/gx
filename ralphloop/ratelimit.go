package ralphloop

import (
	"time"

	"github.com/elentok/gx/agentrunner"
)

// rateLimitPollInterval is how often waitForRateLimitReset re-checks a pane
// when the reset time in its rate-limit message couldn't be parsed.
const rateLimitPollInterval = 5 * time.Minute

// resumePollInterval is how often waitForClaudeRateLimitReset re-checks
// whether label is still paused while waiting out a rate limit.
const resumePollInterval = 2 * time.Second

// rateLimitResetBuffer is added past a successfully-parsed reset time so the
// loop wakes just after the quota actually rolls over, not right at it.
const rateLimitResetBuffer = 60 * time.Second

// codexRateLimitMaxRepolls caps how many times the Codex wait re-checks
// Codex's own quota snapshot for a still-"exhausted" result — past the reset
// deadline, or (with no deadline at all) on the fallback poll — before giving
// up and returning control to the caller. The rollout record Codex wrote
// before hitting its limit is immutable until Codex actually makes a new
// request, so an unchanged "exhausted" snapshot would otherwise poll the
// runner forever and never release the pause; the caller
// (recoverCodexRateLimit) re-observes the pane directly once this returns.
const codexRateLimitMaxRepolls = 3

// rateLimitWait tunes waitForRateLimitReset.
type rateLimitWait struct {
	// paused, when set, is checked every resumePollInterval; the wait ends as
	// soon as it reports false (e.g. an in-process Gate.ForceResume). When
	// set, reaching a known reset deadline also ends the wait without asking
	// the runner. When nil, the wait sleeps straight through each window and
	// always asks the runner once the deadline has passed.
	paused func() bool
	// maxRepolls bounds the runner re-checks after the deadline (or on the
	// fallback poll when resetAt is unknown). Zero means unbounded.
	maxRepolls int
}

// waitForRateLimitReset blocks until the rate limit has (probably) cleared:
// once resetAt (plus rateLimitResetBuffer) has passed, or — with no known
// reset — by asking the runner every rateLimitPollInterval until it no longer
// reports the session limited.
func waitForRateLimitReset(d Deps, s agentrunner.Session, resetAt time.Time, opt rateLimitWait) {
	// sleep waits dur, returning false if opt.paused says to stop early.
	// Polling at resumePollInterval lets a resume be noticed quickly rather
	// than sleeping through the whole reset window.
	sleep := func(dur time.Duration) bool {
		if opt.paused == nil {
			d.Sleep(dur)
			return true
		}
		end := d.Now().Add(dur)
		for {
			if !opt.paused() {
				return false
			}
			if !d.Now().Before(end) {
				return true
			}
			d.Sleep(resumePollInterval)
		}
	}
	cleared := func() bool {
		_, limited, err := d.Runner.RateLimit(s)
		return err == nil && !limited
	}

	if !resetAt.IsZero() {
		if wait := resetAt.Add(rateLimitResetBuffer).Sub(d.Now()); wait > 0 || opt.paused != nil {
			if !sleep(max(wait, 0)) {
				return
			}
		}
		if opt.paused != nil || cleared() {
			return
		}
	}

	for i := 0; opt.maxRepolls == 0 || i < opt.maxRepolls; i++ {
		if !sleep(rateLimitPollInterval) || cleared() {
			return
		}
	}
}

// waitForClaudeRateLimitReset waits until the rate limit has (probably)
// cleared or an in-process Gate.ForceResume resumes label, whichever is first.
func waitForClaudeRateLimitReset(d Deps, g *Gate, s agentrunner.Session, resetAt time.Time) {
	waitForRateLimitReset(d, s, resetAt, rateLimitWait{
		paused: func() bool { return g.isLabelPaused(s.Label) },
	})
}

// waitForCodexRateLimitReset waits for the structured session reset time when
// available; missing or malformed reset data falls back to polling the same
// session observer. A snapshot that keeps reporting "exhausted" is re-checked
// at most codexRateLimitMaxRepolls times, so a stale pre-reset record can't
// hold the pause open indefinitely.
func waitForCodexRateLimitReset(d Deps, s agentrunner.Session, resetAt time.Time) {
	waitForRateLimitReset(d, s, resetAt, rateLimitWait{maxRepolls: codexRateLimitMaxRepolls})
}
