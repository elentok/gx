package codexsession

import (
	"testing"
	"time"
)

func TestLatestUsageIn_ReturnsNewestWindowsAtAnyPercentage(t *testing.T) {
	home := writeCodexHome(t, "rollout-2026-08-01T10-00-00-session-1.jsonl", `{"type":"session_meta","payload":{"id":"session-1","cwd":"/repo/iter-01"}}
{"timestamp":"2026-08-01T10:00:00Z","type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":10,"window_minutes":300,"resets_at":1000},"secondary":null}}}
{"timestamp":"2026-08-01T10:05:00Z","type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":98,"window_minutes":10080,"resets_at":2000},"secondary":null}}}
{"timestamp":"2026-08-01T10:06:00Z","type":"event_msg","payload":{"type":"token_count","info":null}}
`)

	got, ok, err := LatestUsageIn(home, "/repo/iter-01", "session-1")
	if err != nil || !ok {
		t.Fatalf("LatestUsageIn = %+v, %t, %v; want a sample", got, ok, err)
	}
	want := UsageWindow{Minutes: 10080, UsedPercent: 98, ResetsAt: time.Unix(2000, 0)}
	if len(got.Windows) != 1 || got.Windows[0] != want {
		t.Errorf("windows = %+v, want [%+v] (the null secondary is absent)", got.Windows, want)
	}
	if wantSeen := time.Date(2026, 8, 1, 10, 5, 0, 0, time.UTC); !got.SeenAt.Equal(wantSeen) {
		t.Errorf("SeenAt = %s, want %s", got.SeenAt, wantSeen)
	}

	if _, ok, _ := LatestUsageIn(home, "/other", "session-1"); ok {
		t.Error("a rollout of another cwd gave a sample")
	}
}
