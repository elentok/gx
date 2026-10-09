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

// waitForClaudeRateLimitReset blocks label's iteration until whichever comes
// first: the rate limit has (probably) cleared, or an in-process
// Gate.ForceResume (e.g. the TUI) resumes label. It polls at
// resumePollInterval so a resume is noticed quickly, rather than sleeping
// through the whole reset window the way a plain wait would.
//
// Clearing itself is checked at a coarser cadence: if resetAt is known, once
// that time (plus buffer) has passed; otherwise by asking the runner every
// rateLimitPollInterval until it no longer reports the session limited.
func waitForClaudeRateLimitReset(d Deps, g *Gate, s agentrunner.Session, resetAt time.Time) {
	deadline := resetAt.Add(rateLimitResetBuffer)
	lastCheck := d.Now()

	for {
		if !g.isLabelPaused(s.Label) {
			return
		}

		now := d.Now()
		if !resetAt.IsZero() {
			if !now.Before(deadline) {
				return
			}
		} else if now.Sub(lastCheck) >= rateLimitPollInterval {
			lastCheck = now
			if _, limited, err := d.Runner.RateLimit(s); err == nil && !limited {
				return
			}
		}

		d.Sleep(resumePollInterval)
	}
}

// codexRateLimitMaxRepolls caps how many times waitForCodexRateLimitReset
// re-checks Codex's own quota snapshot for a still-"exhausted" result — past
// the reset deadline, or (with no deadline at all) on the fallback poll —
// before giving up and returning control to the caller. The rollout record
// Codex wrote before hitting its limit is immutable until Codex actually
// makes a new request, so an unchanged "exhausted" snapshot would otherwise
// poll the runner forever and never release the pause; the caller
// (recoverCodexRateLimit) re-observes the pane directly once this returns.
const codexRateLimitMaxRepolls = 3

// waitForCodexRateLimitReset waits for the structured session reset time when
// available, using the deadline plus rateLimitResetBuffer as the boundary for
// trying Codex again — never blocking past it. Missing or malformed reset
// data falls back to polling the same session observer instead. Either way,
// a quota snapshot that keeps reporting "exhausted" is re-checked at most
// codexRateLimitMaxRepolls times before this returns regardless, so a stale
// pre-reset record can't hold the pause open indefinitely.
func waitForCodexRateLimitReset(d Deps, s agentrunner.Session, resetAt time.Time) {
	cleared := func() bool {
		_, limited, err := d.Runner.RateLimit(s)
		return err == nil && !limited
	}
	if !resetAt.IsZero() {
		if wait := resetAt.Add(rateLimitResetBuffer).Sub(d.Now()); wait > 0 {
			d.Sleep(wait)
		}
		if cleared() {
			return
		}
	}

	for range codexRateLimitMaxRepolls {
		d.Sleep(rateLimitPollInterval)
		if cleared() {
			return
		}
	}
}
