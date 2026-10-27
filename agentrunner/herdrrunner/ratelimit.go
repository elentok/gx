package herdrrunner

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
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
func ReadPaneRecent(cli herdr.CLI) PaneReader {
	return func(pane string) (string, error) {
		return cli.AgentRead(pane, herdr.AgentReadOptions{Source: "recent-unwrapped"})
	}
}

var (
	codexQuotaLinePattern     = regexp.MustCompile(`(?i)^you(?:('|’)ve| have) hit your usage limit(?:[.!]|$)`)
	codexAbsoluteResetPattern = regexp.MustCompile(`(?i)(?:try again|resets?) at\s+([a-z]+\s+[0-9]{1,2}(?:st|nd|rd|th)?,\s+[0-9]{4}\s+[0-9]{1,2}(?::[0-9]{2})?\s*[ap]m|[0-9]{1,2}(?::[0-9]{2})?\s*[ap]m)`)
	codexRelativeResetPattern = regexp.MustCompile(`(?i)try again in\s+([^.]*)`)
	codexRelativePartPattern  = regexp.MustCompile(`(?i)([0-9]+)\s*(day|hour|minute|second)s?`)
	codexOrdinalPattern       = regexp.MustCompile(`(?i)([0-9]{1,2})(?:st|nd|rd|th)`)
)

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
		if duration, ok := claudeui.SecondsUntilReset(token, now); ok {
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
