package ralphloop

import (
	"time"

	"github.com/elentok/gx/agentrunner/herdrrunner"
	"github.com/elentok/gx/codexsession"
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
// Clearing itself is checked at a coarser cadence: if token parsed to a
// reset time, once that time (plus buffer) has passed; otherwise by
// re-reading pane's recent output every rateLimitPollInterval until the
// rate-limit message is no longer present.
func waitForClaudeRateLimitReset(d Deps, g *Gate, label, pane, token string) {
	deadline, hasDeadline := time.Time{}, false
	if wait, ok := herdrrunner.SecondsUntilReset(token, d.Now()); ok {
		deadline, hasDeadline = d.Now().Add(wait+rateLimitResetBuffer), true
	}
	lastTextCheck := d.Now()

	for {
		if !g.isLabelPaused(label) {
			return
		}

		now := d.Now()
		if hasDeadline {
			if !now.Before(deadline) {
				return
			}
		} else if d.ReadPaneRecent != nil && now.Sub(lastTextCheck) >= rateLimitPollInterval {
			lastTextCheck = now
			if text, err := d.ReadPaneRecent(pane); err == nil {
				if _, matched := herdrrunner.DetectRateLimit(text); !matched {
					return
				}
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
// poll d.ReadCodexRateLimit forever and never release the pause; the caller
// (recoverCodexRateLimit) re-observes the pane directly once this returns.
const codexRateLimitMaxRepolls = 3

// waitForCodexRateLimitReset waits for the structured session reset time when
// available, using the deadline plus rateLimitResetBuffer as the boundary for
// trying Codex again — never blocking past it. Missing or malformed reset
// data falls back to polling the same session observer instead. Either way,
// a quota snapshot that keeps reporting "exhausted" is re-checked at most
// codexRateLimitMaxRepolls times before this returns regardless, so a stale
// pre-reset record can't hold the pause open indefinitely.
func waitForCodexRateLimitReset(d Deps, cwd, sessionID string, limit codexsession.RateLimit) {
	if !limit.ResetAt.IsZero() {
		if wait := limit.ResetAt.Add(rateLimitResetBuffer).Sub(d.Now()); wait > 0 {
			d.Sleep(wait)
		}
		if d.ReadCodexRateLimit == nil {
			return
		}
		_, exhausted, err := d.ReadCodexRateLimit(cwd, sessionID)
		if err == nil && !exhausted {
			return
		}
	}

	for range codexRateLimitMaxRepolls {
		d.Sleep(rateLimitPollInterval)
		if d.ReadCodexRateLimit == nil {
			return
		}
		_, exhausted, err := d.ReadCodexRateLimit(cwd, sessionID)
		if err == nil && !exhausted {
			return
		}
	}
}
