package codexsession

import (
	"bufio"
	"encoding/json"
	"os"
	"time"
)

// usageLimitKind is Codex's error-info name for a hit usage limit. It is not
// in any recording; the 100% sample is the evidence that was seen.
const usageLimitKind = "usage_limit_exceeded"

// TurnError is the error a Codex turn ended in.
type TurnError struct {
	Message string
	// Kind is Codex's error-info name, such as "model_not_found".
	Kind string
}

// TurnErrorReading is a rollout's open turn error plus the newest rate-limit
// sample, which is what tells a usage limit from any other error.
type TurnErrorReading struct {
	Error  TurnError
	sample quotaSample
}

// quotaSample is one token_count line's rate_limits.
type quotaSample struct {
	primary, secondary quotaWindow
	seen               bool
}

// UsageLimit reports whether the error is a usage limit, and when it resets
// (zero when unknown). A window at 100% counts only while its reset has not
// passed; the error kind alone also counts, with an unknown reset. The newest
// sample wins, so a later sample under 100% clears an earlier one.
func (r TurnErrorReading) UsageLimit(now time.Time) (RateLimit, bool) {
	if r.sample.seen {
		if limit, ok := exhaustedQuota(r.sample.primary, "primary"); ok && !resetPassed(limit, now) {
			return limit, true
		}
		if limit, ok := exhaustedQuota(r.sample.secondary, "secondary"); ok && !resetPassed(limit, now) {
			return limit, true
		}
	}
	if r.Error.Kind == usageLimitKind {
		return RateLimit{}, true
	}
	return RateLimit{}, false
}

func resetPassed(limit RateLimit, now time.Time) bool {
	return !limit.ResetAt.IsZero() && !limit.ResetAt.After(now)
}

// LastTurnErrorIn returns the error sessionID's newest turn ended in, launched
// in cwd, read from the Codex home directory. A later turn start or clean
// completion clears it, so ok=false means the session is not in an error.
func LastTurnErrorIn(home, cwd, sessionID string) (reading TurnErrorReading, ok bool, err error) {
	if cwd == "" || sessionID == "" {
		return TurnErrorReading{}, false, nil
	}
	err = walkSessionFiles(home, sessionID, func(path string) error {
		value, valid, readErr := readTurnError(path, cwd, sessionID)
		if readErr != nil {
			return readErr
		}
		if valid {
			reading, ok = value, true
		}
		return nil
	})
	if err != nil {
		return TurnErrorReading{}, false, err
	}
	return reading, ok, nil
}

func readTurnError(path, cwd, sessionID string) (reading TurnErrorReading, ok bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return TurnErrorReading{}, false, err
	}
	defer f.Close()

	matchingSession := false
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var line sessionLine
		if json.Unmarshal(scanner.Bytes(), &line) != nil {
			continue
		}
		if line.Type == "session_meta" {
			matchingSession = line.Payload.ID == sessionID && sameDir(line.Payload.Cwd, cwd)
			continue
		}
		if !matchingSession || line.Type != "event_msg" {
			continue
		}
		switch line.Payload.Type {
		case "task_started":
			ok = false
		case "task_complete":
			e := line.Payload.Error
			ok = e.Message != "" || len(e.Info) > 0
			reading.Error = TurnError{Message: e.Message, Kind: errorKind(e.Info)}
		case "token_count":
			primary, secondary := line.Payload.RateLimits.Primary, line.Payload.RateLimits.Secondary
			if hasQuotaData(primary) || hasQuotaData(secondary) {
				reading.sample = quotaSample{primary: primary, secondary: secondary, seen: true}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return TurnErrorReading{}, false, err
	}
	return reading, ok, nil
}

// errorKind names an error-info value: a bare string, or the single key of an
// object such as {"http_connection_failed":{...}}.
func errorKind(info json.RawMessage) string {
	var name string
	if json.Unmarshal(info, &name) == nil {
		return name
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(info, &object) == nil {
		for key := range object {
			return key
		}
	}
	return ""
}
