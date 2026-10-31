package codexsession

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeCodexHome writes a rollout file under a fresh Codex home and returns the home.
func writeCodexHome(t *testing.T, name, contents string) string {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, "sessions", "2026", "08", "01", name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return home
}

func TestLastContextTokensIn_ReturnsLatestMatchingSessionForWorktree(t *testing.T) {
	home := writeCodexHome(t, "rollout-2026-08-01T10-00-00-session-1.jsonl", `{"type":"session_meta","payload":{"id":"session-1","cwd":"/repo/iter-01"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":120000}}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":151000}}}}
`)

	got, ok, err := LastContextTokensIn(home, "/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("LastContextTokensIn: %v", err)
	}
	if !ok || got != 151000 {
		t.Errorf("LastContextTokensIn() = (%d, %t), want (151000, true)", got, ok)
	}
}

func TestVerifyIdentityIn_RequiresMatchingRolloutMetadata(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions", "2026", "08", "04")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	write := func(name, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(sessionsDir, name), []byte(contents), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	write("rollout-session-good.jsonl", `{"type":"session_meta","payload":{"id":"session-good","cwd":"/repo/iter-01"}}`+"\n")
	write("rollout-session-wrong-id.jsonl", `{"type":"session_meta","payload":{"id":"different","cwd":"/repo/iter-01"}}`+"\n")
	write("rollout-session-wrong-cwd.jsonl", `{"type":"session_meta","payload":{"id":"session-wrong-cwd","cwd":"/repo/other"}}`+"\n")

	for _, tc := range []struct {
		name, cwd, sessionID string
		want                 bool
	}{
		{name: "matching", cwd: "/repo/iter-01", sessionID: "session-good", want: true},
		{name: "missing", cwd: "/repo/iter-01", sessionID: "session-missing"},
		{name: "wrong id", cwd: "/repo/iter-01", sessionID: "session-wrong-id"},
		{name: "wrong cwd", cwd: "/repo/iter-01", sessionID: "session-wrong-cwd"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := VerifyIdentityIn(home, tc.cwd, tc.sessionID)
			if err != nil {
				t.Fatalf("VerifyIdentityIn() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("VerifyIdentityIn() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestLastContextTokensIn_IgnoresMissingOrWrongWorktreeSession(t *testing.T) {
	home := writeCodexHome(t, "rollout-2026-08-01T10-00-00-session-1.jsonl", `{"type":"session_meta","payload":{"id":"session-1","cwd":"/repo/iter-02"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":200000}}}}
`)

	got, ok, err := LastContextTokensIn(home, "/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("LastContextTokensIn: %v", err)
	}
	if ok || got != 0 {
		t.Errorf("LastContextTokensIn() = (%d, %t), want (0, false)", got, ok)
	}
}

func TestLastContextTokensIn_MissingOrMalformedSessionDataDoesNotReportContext(t *testing.T) {
	home := writeCodexHome(t, "rollout-2026-08-01T10-00-00-session-1.jsonl",
		"{not json}\n{\"type\":\"session_meta\",\"payload\":{\"id\":\"session-1\",\"cwd\":\"/repo/iter-01\"}}\n")

	got, ok, err := LastContextTokensIn(home, "/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("LastContextTokensIn: %v", err)
	}
	if ok || got != 0 {
		t.Errorf("LastContextTokensIn() = (%d, %t), want (0, false)", got, ok)
	}
}

func TestLastRateLimitIn_ReturnsExhaustedPrimaryQuotaForMatchingSession(t *testing.T) {
	home := writeCodexHome(t, "rollout-session-1.jsonl", `{"type":"session_meta","payload":{"id":"session-1","cwd":"/repo/iter-01"}}
{"type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":100,"resets_at":1786170140},"secondary":{"used_percent":72,"resets_at":1786200000}}}}
`)

	got, ok, err := LastRateLimitIn(home, "/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("LastRateLimitIn: %v", err)
	}
	if !ok {
		t.Fatal("LastRateLimitIn() ok = false, want true")
	}
	if got.Quota != "primary" || !got.ResetAt.Equal(time.Unix(1786170140, 0)) {
		t.Errorf("LastRateLimitIn() = %+v, want exhausted primary at %v", got, time.Unix(1786170140, 0))
	}
}

func TestLastRateLimitIn_LatestQuotaStateClearsAnEarlierExhaustion(t *testing.T) {
	home := writeCodexHome(t, "rollout-session-1.jsonl", `{"type":"session_meta","payload":{"id":"session-1","cwd":"/repo/iter-01"}}
{"type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":100,"resets_at":1786170140}}}}
{"type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":12,"resets_at":1786170140}}}}
`)

	_, ok, err := LastRateLimitIn(home, "/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("LastRateLimitIn: %v", err)
	}
	if ok {
		t.Error("LastRateLimitIn() ok = true, want false after quota reset")
	}
}

func TestReadStatsInAndLastRateLimitIn_ReadTheSameHome(t *testing.T) {
	home := writeCodexHome(t, "rollout-2026-08-01T10-00-00-session-1.jsonl", `{"type":"session_meta","timestamp":"2026-08-01T10:00:00Z","payload":{"id":"session-1","cwd":"/repo/iter-01"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":151000},"total_token_usage":{"total_tokens":300000}},"rate_limits":{"primary":{"used_percent":100,"resets_at":1786170140}}}}
`)

	stats, ok, err := ReadStatsIn(home, "/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("ReadStatsIn: %v", err)
	}
	if !ok || stats.TotalTokens != 300000 {
		t.Errorf("ReadStatsIn() = (%+v, %t), want TotalTokens=300000, ok=true", stats, ok)
	}

	limit, ok, err := LastRateLimitIn(home, "/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("LastRateLimitIn: %v", err)
	}
	if !ok || limit.Quota != "primary" {
		t.Errorf("LastRateLimitIn() = (%+v, %t), want exhausted primary, ok=true", limit, ok)
	}
}

func TestReaders_MatchARolloutThroughASymlinkedCwd(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	home := writeCodexHome(t, "rollout-session-1.jsonl", `{"type":"session_meta","timestamp":"2026-08-01T10:00:00Z","payload":{"id":"session-1","cwd":"`+real+`"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":151000},"total_token_usage":{"total_tokens":300000}},"rate_limits":{"primary":{"used_percent":100,"resets_at":1786170140}}}}
`)

	if tokens, ok, err := LastContextTokensIn(home, link, "session-1"); err != nil || !ok || tokens != 151000 {
		t.Errorf("LastContextTokensIn = (%d, %t, %v), want 151000", tokens, ok, err)
	}
	if ok, err := VerifyIdentityIn(home, link, "session-1"); err != nil || !ok {
		t.Errorf("VerifyIdentityIn = (%t, %v), want true", ok, err)
	}
	if stats, ok, err := ReadStatsIn(home, link, "session-1"); err != nil || !ok || stats.TotalTokens != 300000 {
		t.Errorf("ReadStatsIn = (%+v, %t, %v), want TotalTokens 300000", stats, ok, err)
	}
	if limit, ok, err := LastRateLimitIn(home, link, "session-1"); err != nil || !ok || limit.Quota != "primary" {
		t.Errorf("LastRateLimitIn = (%+v, %t, %v), want primary", limit, ok, err)
	}
}

func TestLastContextTokensIn_MissingHomeDirFailsSafe(t *testing.T) {
	got, ok, err := LastContextTokensIn(filepath.Join(t.TempDir(), "does-not-exist"), "/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("LastContextTokensIn: %v", err)
	}
	if ok || got != 0 {
		t.Errorf("LastContextTokensIn() = (%d, %t), want (0, false)", got, ok)
	}
}
