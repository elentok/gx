package transcript

import (
	"path/filepath"
	"testing"
	"time"
)

// Recorded from claude transcripts, trimmed to the fields gx reads.
const (
	replyLine     = `{"type":"assistant","timestamp":"2026-10-11T07:00:00.000Z","message":{"role":"assistant","content":[{"type":"text","text":"Done."}]}}`
	userLine      = `{"type":"user","timestamp":"2026-10-11T07:01:00.000Z","message":{"role":"user","content":"go on"}}`
	sessionLimit  = `{"type":"assistant","timestamp":"2026-10-11T07:02:00.000Z","isApiErrorMessage":true,"error":"rate_limit","apiErrorStatus":429,"message":{"role":"assistant","content":[{"type":"text","text":"You've hit your session limit · resets 11pm (Asia/Jerusalem)"}]}}`
	bare429       = `{"type":"assistant","timestamp":"2026-10-11T07:02:00.000Z","isApiErrorMessage":true,"error":"unknown","apiErrorStatus":429,"message":{"content":"API Error: 429"}}`
	serverError   = `{"type":"assistant","timestamp":"2026-10-11T07:02:00.000Z","isApiErrorMessage":true,"error":"server_error","apiErrorStatus":null,"message":{"content":[{"type":"text","text":"API Error: Your computer went to sleep mid-response."}]}}`
	retryInFlight = `{"type":"system","subtype":"api_error","level":"error","timestamp":"2026-10-11T07:03:00.000Z","error":{"status":529},"retryInMs":531,"retryAttempt":1}`
)

func TestLastAPIError(t *testing.T) {
	at := time.Date(2026, 10, 11, 7, 2, 0, 0, time.UTC)
	cases := []struct {
		name        string
		lines       []string
		want        APIError
		wantOK      bool
		rateLimited bool
	}{
		{"rate_limit with a 429", []string{replyLine, userLine, sessionLimit}, APIError{Error: "rate_limit", Status: 429, Message: "You've hit your session limit · resets 11pm (Asia/Jerusalem)", Timestamp: at}, true, true},
		{"a bare 429", []string{bare429}, APIError{Error: "unknown", Status: 429, Message: "API Error: 429", Timestamp: at}, true, true},
		{"other API error", []string{serverError}, APIError{Error: "server_error", Message: "API Error: Your computer went to sleep mid-response.", Timestamp: at}, true, false},
		{"a retry after the error is not a reply", []string{sessionLimit, userLine, retryInFlight}, APIError{Error: "rate_limit", Status: 429, Message: "You've hit your session limit · resets 11pm (Asia/Jerusalem)", Timestamp: at}, true, true},
		{"a later successful turn", []string{sessionLimit, userLine, replyLine}, APIError{}, false, false},
		{"no assistant line", []string{userLine}, APIError{}, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok, err := LastAPIError(writeTranscript(t, c.lines...))
			if err != nil {
				t.Fatal(err)
			}
			if ok != c.wantOK || got != c.want {
				t.Fatalf("LastAPIError = %+v, %v; want %+v, %v", got, ok, c.want, c.wantOK)
			}
			if got.RateLimited() != c.rateLimited {
				t.Fatalf("RateLimited = %v, want %v", got.RateLimited(), c.rateLimited)
			}
		})
	}
}

func TestLastAPIError_MissingFile(t *testing.T) {
	if _, ok, err := LastAPIError(filepath.Join(t.TempDir(), "none.jsonl")); ok || err != nil {
		t.Fatalf("missing file: ok=%v err=%v", ok, err)
	}
}
