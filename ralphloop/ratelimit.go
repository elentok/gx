package ralphloop

import (
	"regexp"
	"strconv"
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

var (
	codexQuotaLinePattern     = regexp.MustCompile(`(?i)^you(?:('|’)ve| have) hit your usage limit(?:[.!]|$)`)
	codexAbsoluteResetPattern = regexp.MustCompile(`(?i)(?:try again|resets?) at\s+([a-z]+\s+[0-9]{1,2}(?:st|nd|rd|th)?,\s+[0-9]{4}\s+[0-9]{1,2}(?::[0-9]{2})?\s*[ap]m|[0-9]{1,2}(?::[0-9]{2})?\s*[ap]m)`)
	codexRelativeResetPattern = regexp.MustCompile(`(?i)try again in\s+([^.]*)`)
	codexRelativePartPattern  = regexp.MustCompile(`(?i)([0-9]+)\s*(day|hour|minute|second)s?`)
	codexOrdinalPattern       = regexp.MustCompile(`(?i)([0-9]{1,2})(?:st|nd|rd|th)`)
)

// detectRateLimit reports whether text contains a Claude usage/session
// rate-limit message and, if so, the reset-time token embedded in it (empty
// if the message didn't include one).
func detectRateLimit(text string) (token string, matched bool) {
	if !rateLimitMessagePattern.MatchString(text) {
		return "", false
	}
	return resetTimeTokenPattern.FindString(text), true
}

func detectCodexRateLimit(text string, now time.Time) (codexsession.RateLimit, bool) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "│┆>*•■🖐"))
		if !codexQuotaLinePattern.MatchString(line) {
			continue
		}

		return codexsession.RateLimit{
			Quota:   "usage",
			ResetAt: codexQuotaResetAt(line, now),
		}, true
	}
	return codexsession.RateLimit{}, false
}

func codexQuotaResetAt(line string, now time.Time) time.Time {
	if match := codexAbsoluteResetPattern.FindStringSubmatch(line); len(match) == 2 {
		token := codexOrdinalPattern.ReplaceAllString(match[1], "$1")
		for _, layout := range []string{
			"Jan 2, 2006 3:04 PM",
			"Jan 2, 2006 3 PM",
			"January 2, 2006 3:04 PM",
			"January 2, 2006 3 PM",
		} {
			if reset, err := time.ParseInLocation(layout, token, time.UTC); err == nil {
				return reset
			}
		}
		if duration, ok := secondsUntilReset(token, now); ok {
			return now.Add(duration)
		}
	}

	match := codexRelativeResetPattern.FindStringSubmatch(line)
	if len(match) != 2 {
		return time.Time{}
	}
	var duration time.Duration
	for _, part := range codexRelativePartPattern.FindAllStringSubmatch(match[1], -1) {
		value, err := strconv.Atoi(part[1])
		if err != nil {
			return time.Time{}
		}
		switch strings.ToLower(part[2]) {
		case "day":
			duration += time.Duration(value) * 24 * time.Hour
		case "hour":
			duration += time.Duration(value) * time.Hour
		case "minute":
			duration += time.Duration(value) * time.Minute
		case "second":
			duration += time.Duration(value) * time.Second
		}
	}
	if duration <= 0 {
		return time.Time{}
	}
	return now.Add(duration)
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
