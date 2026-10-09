package herdrrunner

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/herdr"
)

// CodexQuotaReader returns an exhausted Codex quota for the session from
// Codex's own rollout records.
type CodexQuotaReader func(cwd, sessionID string) (limit codexsession.RateLimit, exhausted bool, err error)

// PaneReader returns a pane's recent terminal output.
type PaneReader func(pane string) (string, error)

// ReadPaneRecent reads a pane's recent unwrapped output (matching
// claude-box's own rate-limit detection source) so long lines aren't split
// mid-word by terminal wrapping before the regex sees them.
func ReadPaneRecent(pane string) (string, error) {
	return herdr.AgentRead(pane, herdr.AgentReadOptions{Source: "recent-unwrapped"})
}

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

var (
	codexQuotaLinePattern     = regexp.MustCompile(`(?i)^you(?:('|’)ve| have) hit your usage limit(?:[.!]|$)`)
	codexAbsoluteResetPattern = regexp.MustCompile(`(?i)(?:try again|resets?) at\s+([a-z]+\s+[0-9]{1,2}(?:st|nd|rd|th)?,\s+[0-9]{4}\s+[0-9]{1,2}(?::[0-9]{2})?\s*[ap]m|[0-9]{1,2}(?::[0-9]{2})?\s*[ap]m)`)
	codexRelativeResetPattern = regexp.MustCompile(`(?i)try again in\s+([^.]*)`)
	codexRelativePartPattern  = regexp.MustCompile(`(?i)([0-9]+)\s*(day|hour|minute|second)s?`)
	codexOrdinalPattern       = regexp.MustCompile(`(?i)([0-9]{1,2})(?:st|nd|rd|th)`)
)

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

func DetectCodexRateLimit(text string, now time.Time) (codexsession.RateLimit, bool) {
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
		if duration, ok := SecondsUntilReset(token, now); ok {
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

// CodexQuotaOrContextExhaustion classifies a Codex session as quota- or
// context-window-exhausted. Codex's own quota record wins; the pane is read
// at most once, so a transient banner can't clear between a quota check and
// a context check. evidence is the pane line showing context exhaustion.
// Nil readers are skipped.
func CodexQuotaOrContextExhaustion(readQuota CodexQuotaReader, readPane PaneReader, now time.Time, cwd, sessionID, pane string) (limit codexsession.RateLimit, exhausted bool, evidence string, err error) {
	if sessionID != "" && readQuota != nil {
		limit, exhausted, err = readQuota(cwd, sessionID)
		if err != nil || exhausted {
			return limit, exhausted, "", err
		}
	}
	if readPane == nil {
		return codexsession.RateLimit{}, false, "", nil
	}
	text, err := readPane(pane)
	if err != nil {
		return codexsession.RateLimit{}, false, "", err
	}
	if quotaLimit, matched := DetectCodexRateLimit(text, now); matched {
		return quotaLimit, true, "", nil
	}
	evidence, _ = DetectCodexContextExhaustion(text)
	return codexsession.RateLimit{}, false, evidence, nil
}

func DetectCodexContextExhaustion(text string) (string, bool) {
	for _, line := range strings.Split(text, "\n") {
		evidence := strings.TrimSpace(line)
		lower := strings.ToLower(evidence)
		structuredCode := strings.Contains(lower, `"code"`) && strings.Contains(lower, "context_length_exceeded")
		streamFailure := strings.Contains(lower, "stream disconnected before completion") &&
			strings.Contains(lower, "your input exceeds the context window of this model")
		terminalFailure := strings.HasPrefix(strings.TrimLeft(lower, "■⚠️ \t"), "codex ran out of room in the model's context window")
		if structuredCode || streamFailure || terminalFailure {
			return evidence, true
		}
	}
	return "", false
}
