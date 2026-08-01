package ralphloop

import (
	"regexp"
	"strings"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/herdr"
)

// rateLimitPollInterval is how often waitForRateLimitReset re-checks a pane
// when the reset time in its rate-limit message couldn't be parsed.
const rateLimitPollInterval = 5 * time.Minute

// rateLimitResetBuffer is added past a successfully-parsed reset time so the
// loop wakes just after the quota actually rolls over, not right at it.
const rateLimitResetBuffer = 60 * time.Second

// rateLimitMessagePattern matches Claude Code's own rate-limit/usage-limit
// messages (e.g. "You've hit your session limit · resets 10:10am (UTC)",
// "Claude usage limit reached") without tripping on incidental mentions like
// "Added a rate limit of 100 requests per minute". This is distinct from a
// generic `blocked` agent status, which can also mean an ordinary permission
// prompt. Ported from claude-box's beads_loop.sh bl_detect_session_limit.
var rateLimitMessagePattern = regexp.MustCompile(`(?i)(hit|reached|reset)[^.]{0,20}(session|usage) limit|(session|usage) limit[^.]{0,40}(hit|reached|reset)`)

// resetTimeTokenPattern extracts a matched rate-limit message's reset clock
// time token, e.g. "10:10am".
var resetTimeTokenPattern = regexp.MustCompile(`(?i)[0-9]{1,2}(:[0-9]{2})?\s*[ap]m`)

// detectRateLimit reports whether text contains a Claude usage/session
// rate-limit message and, if so, the reset-time token embedded in it (empty
// if the message didn't include one).
func detectRateLimit(text string) (token string, matched bool) {
	if !rateLimitMessagePattern.MatchString(text) {
		return "", false
	}
	return resetTimeTokenPattern.FindString(text), true
}

// secondsUntilReset parses token (a bare clock time, e.g. "10:10am" or
// "3pm", assumed UTC) and returns the duration from now until its next
// occurrence, rolling to the next day if that time has already passed today.
// ok is false if token is empty or unparseable, so callers can fall back to
// fixed-interval polling.
func secondsUntilReset(token string, now time.Time) (d time.Duration, ok bool) {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(token), " ", ""))
	if normalized == "" {
		return 0, false
	}

	nowUTC := now.UTC()
	for _, layout := range []string{"3:04pm", "3pm"} {
		t, err := time.Parse(layout, normalized)
		if err != nil {
			continue
		}
		target := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), t.Hour(), t.Minute(), 0, 0, time.UTC)
		if !target.After(nowUTC) {
			target = target.AddDate(0, 0, 1)
		}
		return target.Sub(nowUTC), true
	}
	return 0, false
}

// defaultReadPaneRecent implements Deps.ReadPaneRecent against the real
// herdr socket API, reading a pane's recent unwrapped output (matching
// claude-box's own rate-limit detection source) so long lines aren't split
// mid-word by terminal wrapping before the regex sees them.
func defaultReadPaneRecent(pane string) (string, error) {
	return herdr.AgentRead(pane, herdr.AgentReadOptions{Source: "recent-unwrapped"})
}

// waitForClaudeRateLimitReset blocks label's iteration until whichever comes
// first: the rate limit has (probably) cleared, or a resume is requested —
// either the file signal a headless `gx ralph-loop resume` writes, or an
// in-process Gate.ForceResume (e.g. the TUI). It polls at resumePollInterval
// so a resume is noticed quickly, rather than sleeping through the whole
// reset window the way a plain wait would.
//
// Clearing itself is checked at a coarser cadence: if token parsed to a
// reset time, once that time (plus buffer) has passed; otherwise by
// re-reading pane's recent output every rateLimitPollInterval until the
// rate-limit message is no longer present.
func waitForClaudeRateLimitReset(d Deps, g *Gate, label, resumeSignalPath, pane, token string) {
	deadline, hasDeadline := time.Time{}, false
	if wait, ok := secondsUntilReset(token, d.Now()); ok {
		deadline, hasDeadline = d.Now().Add(wait+rateLimitResetBuffer), true
	}
	lastTextCheck := d.Now()

	for {
		if !g.isLabelPaused(label) {
			return
		}
		if signaled, err := d.ResumeSignaled(resumeSignalPath); err == nil && signaled {
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
				if _, matched := detectRateLimit(text); !matched {
					return
				}
			}
		}

		d.Sleep(resumePollInterval)
	}
}

// waitForCodexRateLimitReset waits for the structured session reset time when
// available. Missing or malformed reset data falls back to polling the same
// session observer until it no longer reports an exhausted quota.
func waitForCodexRateLimitReset(d Deps, cwd, sessionID string, limit codexsession.RateLimit) {
	if !limit.ResetAt.IsZero() {
		if wait := time.Until(limit.ResetAt); wait > 0 {
			d.Sleep(wait + rateLimitResetBuffer)
		}
		if d.ReadCodexRateLimit == nil {
			return
		}
		_, exhausted, err := d.ReadCodexRateLimit(cwd, sessionID)
		if err == nil && !exhausted {
			return
		}
	}

	for {
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
