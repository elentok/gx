package codexsession

import (
	"fmt"
	"os"
	"path/filepath"
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
			reading, ok, err := LastTurnErrorIn(home, "/repo", "s1", time.Time{})
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
	reading, ok, err := LastTurnErrorIn(home, "/repo", "s1", time.Time{})
	if err != nil || !ok || reading.Error != (TurnError{Message: "model gone", Kind: "model_not_found"}) {
		t.Fatalf("LastTurnErrorIn = %+v, %v, %v", reading, ok, err)
	}

	home = writeCodexHome(t, "rollout-s1.jsonl", meta+failed+`{"type":"event_msg","payload":{"type":"task_started"}}`+"\n")
	if _, ok, err := LastTurnErrorIn(home, "/repo", "s1", time.Time{}); err != nil || ok {
		t.Errorf("after a new turn: ok=%v err=%v, want no error", ok, err)
	}
}

func TestLastTurnErrorIn_IgnoresAModelNotFoundTurnOlderThanTheSwitch(t *testing.T) {
	meta := `{"type":"session_meta","payload":{"id":"s1","cwd":"/repo"}}` + "\n"
	failed := func(ts, kind string) string {
		return `{"timestamp":"` + ts + `","type":"event_msg","payload":{"type":"task_complete","error":{"message":"boom","codex_error_info":"` + kind + `"}}}` + "\n"
	}
	switched := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, rollout string
		switchedAt    time.Time
		want          bool
	}{
		{"older model_not_found", failed("2026-11-01T11:59:00Z", "model_not_found"), switched, false},
		{"newer model_not_found", failed("2026-11-01T12:01:00Z", "model_not_found"), switched, true},
		{"older other error stays", failed("2026-11-01T11:59:00Z", "other"), switched, true},
		{"no switch time", failed("2026-11-01T11:59:00Z", "model_not_found"), time.Time{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := writeCodexHome(t, "rollout-s1.jsonl", meta+tc.rollout)
			if _, ok, err := LastTurnErrorIn(home, "/repo", "s1", tc.switchedAt); err != nil || ok != tc.want {
				t.Errorf("ok=%v err=%v, want %v", ok, err, tc.want)
			}
		})
	}
}

func TestFirstTurnEndAfter(t *testing.T) {
	at := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	done := func(ts, info string) string {
		return `{"timestamp":"` + ts + `","type":"event_msg","payload":{"type":"task_complete","error":{"message":"m","codex_error_info":` + info + `}}}` + "\n"
	}
	started := `{"timestamp":"2026-11-01T12:00:30Z","type":"event_msg","payload":{"type":"task_started"}}` + "\n"
	clean := `{"timestamp":"2026-11-01T12:01:00Z","type":"event_msg","payload":{"type":"task_complete"}}` + "\n"
	for _, tc := range []struct {
		name, rollout, wantKind string
		wantFound               bool
	}{
		{"nothing yet", "", "", false},
		{"turn still running", started, "", false},
		{"older completion ignored", done("2026-11-01T11:00:00Z", `"model_not_found"`), "", false},
		{"model_not_found", started + done("2026-11-01T12:01:00Z", `"model_not_found"`), ModelNotFound, true},
		{"other error", started + done("2026-11-01T12:01:00Z", `"rate_limit_exceeded"`), "rate_limit_exceeded", true},
		{"clean completion", started + clean, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "rollout.jsonl")
			if err := os.WriteFile(path, []byte(tc.rollout), 0o644); err != nil {
				t.Fatal(err)
			}
			kind, found, err := FirstTurnEndAfter(path, at)
			if err != nil || found != tc.wantFound || kind != tc.wantKind {
				t.Errorf("= %q, %v, %v; want %q, %v", kind, found, err, tc.wantKind, tc.wantFound)
			}
		})
	}
}

func TestErrorKind_TakesAnObjectsKey(t *testing.T) {
	if got := errorKind([]byte(`{"http_connection_failed":{"http_status_code":502}}`)); got != "http_connection_failed" {
		t.Errorf("errorKind = %q", got)
	}
}
