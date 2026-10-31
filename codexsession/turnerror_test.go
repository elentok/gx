package codexsession

import (
	"fmt"
	"testing"
	"time"
)

func TestLastTurnErrorIn_ClassifiesUsageLimitFromTheNewestSample(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	future, past := now.Add(time.Hour).Unix(), now.Add(-time.Hour).Unix()
	meta := `{"type":"session_meta","payload":{"id":"s1","cwd":"/repo"}}` + "\n"
	sample := func(percent float64, resetsAt int64) string {
		return fmt.Sprintf(`{"type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":%v,"window_minutes":300,"resets_at":%d}}}}`+"\n", percent, resetsAt)
	}
	failed := func(message, kind string) string {
		return fmt.Sprintf(`{"type":"event_msg","payload":{"type":"task_complete","error":{"message":%q,"codex_error_info":%q}}}`+"\n", message, kind)
	}

	for _, tc := range []struct {
		name      string
		rollout   string
		wantLimit bool
		wantReset time.Time
	}{
		{"100% sample, reset ahead", sample(100, future) + failed("boom", "other"), true, time.Unix(future, 0)},
		{"100% sample, reset passed", sample(100, past) + failed("boom", "other"), false, time.Time{}},
		{"usage limit kind, no sample", failed("limit", "usage_limit_exceeded"), true, time.Time{}},
		{"unrelated kind", failed("no such model", "model_not_found"), false, time.Time{}},
		{"error, no sample", failed("boom", ""), false, time.Time{}},
		{"later 54% sample clears the 100% one", sample(100, future) + sample(54, future) + failed("boom", "other"), false, time.Time{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := writeCodexHome(t, "rollout-s1.jsonl", meta+tc.rollout)
			reading, ok, err := LastTurnErrorIn(home, "/repo", "s1")
			if err != nil || !ok {
				t.Fatalf("LastTurnErrorIn = %v, %v, want an error", ok, err)
			}
			limit, isLimit := reading.UsageLimit(now)
			if isLimit != tc.wantLimit || !limit.ResetAt.Equal(tc.wantReset) {
				t.Errorf("UsageLimit = %+v, %v; want limit=%v reset=%v", limit, isLimit, tc.wantLimit, tc.wantReset)
			}
		})
	}
}

func TestLastTurnErrorIn_ReadsTheMessageAndClearsOnTheNextTurn(t *testing.T) {
	meta := `{"type":"session_meta","payload":{"id":"s1","cwd":"/repo"}}` + "\n"
	failed := `{"type":"event_msg","payload":{"type":"task_complete","error":{"message":"model gone","codex_error_info":"model_not_found"}}}` + "\n"

	home := writeCodexHome(t, "rollout-s1.jsonl", meta+failed)
	reading, ok, err := LastTurnErrorIn(home, "/repo", "s1")
	if err != nil || !ok || reading.Error != (TurnError{Message: "model gone", Kind: "model_not_found"}) {
		t.Fatalf("LastTurnErrorIn = %+v, %v, %v", reading, ok, err)
	}

	home = writeCodexHome(t, "rollout-s1.jsonl", meta+failed+`{"type":"event_msg","payload":{"type":"task_started"}}`+"\n")
	if _, ok, err := LastTurnErrorIn(home, "/repo", "s1"); err != nil || ok {
		t.Errorf("after a new turn: ok=%v err=%v, want no error", ok, err)
	}
}

func TestErrorKind_TakesAnObjectsKey(t *testing.T) {
	if got := errorKind([]byte(`{"http_connection_failed":{"http_status_code":502}}`)); got != "http_connection_failed" {
		t.Errorf("errorKind = %q", got)
	}
}
