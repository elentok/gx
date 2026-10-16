package transcript

import (
	"encoding/json"
	"slices"
	"time"
)

// APIError is an assistant line claude writes in place of a reply when the
// API call failed for good; retries in flight are system lines, not these.
type APIError struct {
	// Error is claude's error kind, e.g. "rate_limit" or "server_error".
	Error string
	// Status is the HTTP status, 0 when claude recorded none.
	Status    int
	Message   string
	Timestamp time.Time
}

// RateLimited reports whether e is a rate or usage limit.
func (e APIError) RateLimited() bool {
	return e.Error == "rate_limit" || e.Status == 429
}

type apiErrorLine struct {
	Type       string `json:"type"`
	Timestamp  string `json:"timestamp"`
	IsAPIError bool   `json:"isApiErrorMessage"`
	Error      string `json:"error"`
	Status     int    `json:"apiErrorStatus"`
	Message    struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// LastAPIError returns the transcript's last assistant line when it is an API
// error; ok is false when that line is a reply, so a later successful turn
// ends the error.
func LastAPIError(path string) (e APIError, ok bool, err error) {
	err = tailScan(path, func(lines []string) bool {
		for _, raw := range slices.Backward(lines) {
			var line apiErrorLine
			if json.Unmarshal([]byte(raw), &line) != nil || line.Type != "assistant" {
				continue
			}
			if line.IsAPIError {
				ts, _ := time.Parse(time.RFC3339Nano, line.Timestamp)
				e, ok = APIError{Error: line.Error, Status: line.Status, Message: contentText(line.Message.Content), Timestamp: ts}, true
			}
			return true
		}
		return false
	})
	if err != nil {
		return APIError{}, false, err
	}
	return e, ok, nil
}

// contentText is a message's text: content is a string or a list of blocks.
func contentText(content json.RawMessage) string {
	var s string
	if json.Unmarshal(content, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	_ = json.Unmarshal(content, &blocks)
	for _, b := range blocks {
		if b.Type == "text" {
			return b.Text
		}
	}
	return ""
}
