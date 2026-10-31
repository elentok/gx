package codexsession

import (
	"os"
	"strings"
	"testing"
	"time"
)

const twoCompactions = `{"timestamp":"2026-08-01T10:00:00Z","type":"session_meta","payload":{"id":"session-1","cwd":"/repo/iter-01"}}
{"timestamp":"2026-08-01T10:01:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":120000}}}}
{"timestamp":"2026-08-01T10:02:00Z","type":"compacted","payload":{"message":""}}
{"timestamp":"2026-08-01T10:03:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":20000}}}}
{"timestamp":"2026-08-01T10:04:00Z","type":"compacted","payload":{"message":""}}
`

func TestCountCompactionsIn_CountsCompactedItems(t *testing.T) {
	home := writeCodexHome(t, "rollout-2026-08-01T10-00-00-session-1.jsonl", twoCompactions)

	got, ok, err := CountCompactionsIn(home, "/repo/iter-01", "session-1")
	if err != nil || !ok || got != 2 {
		t.Errorf("CountCompactionsIn() = (%d, %t, %v), want (2, true, nil)", got, ok, err)
	}
}

func TestCountCompactionsAfterIn_CountsAtOrAfterSince(t *testing.T) {
	home := writeCodexHome(t, "rollout-2026-08-01T10-00-00-session-1.jsonl", twoCompactions)

	cases := []struct {
		since time.Time
		want  int
	}{
		{time.Date(2026, 8, 1, 10, 3, 0, 0, time.UTC), 1},
		{time.Date(2026, 8, 1, 10, 4, 0, 0, time.UTC), 1}, // at counts
		{time.Date(2026, 8, 1, 10, 4, 1, 0, time.UTC), 0},
		{time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC), 2},
	}
	for _, tc := range cases {
		got, ok, err := CountCompactionsAfterIn(home, "/repo/iter-01", "session-1", tc.since)
		if err != nil || !ok || got != tc.want {
			t.Errorf("CountCompactionsAfterIn(%s) = (%d, %t, %v), want (%d, true, nil)", tc.since, got, ok, err, tc.want)
		}
	}
}

func TestCountCompactionsIn_UnreadableIsNotOK(t *testing.T) {
	t.Run("no rollout file", func(t *testing.T) {
		got, ok, err := CountCompactionsIn(t.TempDir(), "/repo/iter-01", "session-1")
		if err != nil || ok || got != 0 {
			t.Errorf("= (%d, %t, %v), want (0, false, nil)", got, ok, err)
		}
	})
	t.Run("rollout for another cwd", func(t *testing.T) {
		home := writeCodexHome(t, "rollout-2026-08-01T10-00-00-session-1.jsonl", twoCompactions)
		got, ok, err := CountCompactionsIn(home, "/repo/other", "session-1")
		if err != nil || ok || got != 0 {
			t.Errorf("= (%d, %t, %v), want (0, false, nil)", got, ok, err)
		}
	})
	t.Run("empty session id", func(t *testing.T) {
		home := writeCodexHome(t, "rollout-2026-08-01T10-00-00-session-1.jsonl", twoCompactions)
		if _, ok, _ := CountCompactionsIn(home, "/repo/iter-01", ""); ok {
			t.Error("ok = true, want false")
		}
	})
}

func TestCountCompactionsIn_FixtureCountsOne(t *testing.T) {
	raw, err := os.ReadFile("../docs/specs/codex-support-fixtures/rollouts/compaction.jsonl")
	if err != nil {
		t.Fatalf("ReadFile fixture: %v", err)
	}
	home := writeCodexHome(t, "rollout-2026-10-30T22-26-39-01a18e7e-24e8-7c02-81bc-bb558b6e733b.jsonl", string(raw))

	got, ok, err := CountCompactionsIn(home, "/Users/david/.cache/gx-codex-rec", "01a18e7e-24e8-7c02-81bc-bb558b6e733b")
	if err != nil || !ok || got != 1 {
		t.Errorf("CountCompactionsIn(fixture) = (%d, %t, %v), want (1, true, nil); lines=%d", got, ok, err, strings.Count(string(raw), "\n"))
	}
}
