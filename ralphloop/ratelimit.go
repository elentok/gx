package ralphloop

import (
	"regexp"
	"strings"
	"time"

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

// waitForRateLimitReset blocks until pane's rate limit has (probably)
// cleared: if token parsed to a reset time, it sleeps straight through to
// just past it; otherwise it polls pane's recent output at
// rateLimitPollInterval until the rate-limit message is no longer present.
func waitForRateLimitReset(d Deps, pane, token string) {
	if wait, ok := secondsUntilReset(token, time.Now()); ok {
		d.Sleep(wait + rateLimitResetBuffer)
		return
	}

	for {
		d.Sleep(rateLimitPollInterval)
		if d.ReadPaneRecent == nil {
			return
		}
		text, err := d.ReadPaneRecent(pane)
		if err != nil {
			continue
		}
		if _, matched := detectRateLimit(text); !matched {
			return
		}
	}
}
