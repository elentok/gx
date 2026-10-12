// Package claudeui reads Claude Code's own UI: its screen text and its
// transcript. It is runner-neutral, so any agentrunner adapter can use it.
package claudeui

import (
	"regexp"
	"strings"
	"time"
)

// rateLimitMessagePattern matches Claude Code's own "limit hit" messages
// (e.g. "You've hit your session limit · resets 10:10am (UTC)", "Claude usage
// limit reached", "5-hour limit reached ∙ resets 3pm") without tripping on
// incidental mentions like "Added a rate limit of 100 requests per minute",
// or on Claude Code's near-limit warnings ("Approaching usage limit · resets
// at 10pm", "You've used 90% of your session limit · resets 10:50pm"), which
// show while the agent can still work. This is distinct from a generic
// `blocked` agent status, which can also mean an ordinary permission prompt.
var rateLimitMessagePattern = regexp.MustCompile(`(?i)\bhit your (session|usage|weekly|[0-9]+-hour) limit|\b(session|usage|weekly|[0-9]+-hour) limit reached`)

// resetTimeTokenPattern extracts a matched rate-limit line's reset clock
// time, plus its zone suffix if it has one, e.g. "10:10am (UTC)" from
// "resets 10:10am (UTC)" or "3pm" from "will reset at 3pm".
var resetTimeTokenPattern = regexp.MustCompile(`(?i)\bresets?\s+(?:at\s+)?([0-9]{1,2}(?::[0-9]{2})?\s*[ap]m(?:\s*\([^)]+\))?)`)

// resetZonePattern splits a reset token into its clock time and its
// optional "(Zone/Name)" suffix.
var resetZonePattern = regexp.MustCompile(`^(.*?)\s*\(([^)]+)\)$`)

// DetectRateLimit reports whether text contains a Claude usage/session
// rate-limit hit and, if so, the reset-time token on that same line (empty
// if the line didn't include one). Only the hit line is searched, so other
// clock times in the pane (e.g. a status line's "done 6:49 PM") are never
// taken for the reset time.
func DetectRateLimit(text string) (token string, matched bool) {
	for _, line := range strings.Split(text, "\n") {
		loc := rateLimitMessagePattern.FindStringIndex(line)
		if loc == nil {
			continue
		}
		if m := resetTimeTokenPattern.FindStringSubmatch(line[loc[1]:]); m != nil {
			return m[1], true
		}
		return "", true
	}
	return "", false
}

// SecondsUntilReset parses token (a clock time, e.g. "10:10am" or "3pm",
// optionally followed by a zone, e.g. "10:10am (UTC)" or
// "3pm (America/New_York)") and returns the duration from now until its next
// occurrence, rolling to the next day if that time has already passed today.
// A token without a zone, or with one that doesn't load, is read in now's
// zone: Claude Code prints reset times in the machine's local time. ok is
// false if token is empty or unparseable, so callers can fall back to
// fixed-interval polling.
func SecondsUntilReset(token string, now time.Time) (d time.Duration, ok bool) {
	clock, loc := strings.TrimSpace(token), now.Location()
	if m := resetZonePattern.FindStringSubmatch(clock); m != nil {
		clock = m[1]
		if zone, err := time.LoadLocation(strings.TrimSpace(m[2])); err == nil {
			loc = zone
		}
	}
	normalized := strings.ToLower(strings.ReplaceAll(clock, " ", ""))
	if normalized == "" {
		return 0, false
	}

	nowIn := now.In(loc)
	for _, layout := range []string{"3:04pm", "3pm"} {
		t, err := time.Parse(layout, normalized)
		if err != nil {
			continue
		}
		target := time.Date(nowIn.Year(), nowIn.Month(), nowIn.Day(), t.Hour(), t.Minute(), 0, 0, loc)
		if !target.After(nowIn) {
			target = target.AddDate(0, 0, 1)
		}
		return target.Sub(nowIn), true
	}
	return 0, false
}
