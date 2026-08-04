package codexsession

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLastContextTokens_ReturnsLatestMatchingSessionForWorktree(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", "")
	root, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	path := filepath.Join(root, ".codex", "sessions", "2026", "08", "01", "rollout-2026-08-01T10-00-00-session-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	contents := `{"type":"session_meta","payload":{"id":"session-1","cwd":"/repo/iter-01"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":120000}}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":151000}}}}
`
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, ok, err := LastContextTokens("/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("LastContextTokens: %v", err)
	}
	if !ok || got != 151000 {
		t.Errorf("LastContextTokens() = (%d, %t), want (151000, true)", got, ok)
	}
}

func TestVerifyIdentity_RequiresMatchingRolloutMetadata(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	sessionsDir := filepath.Join(os.Getenv("CODEX_HOME"), "sessions", "2026", "08", "04")
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
			got, err := VerifyIdentity(tc.cwd, tc.sessionID)
			if err != nil {
				t.Fatalf("VerifyIdentity() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("VerifyIdentity() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestLastContextTokens_IgnoresMissingOrWrongWorktreeSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", "")
	root, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	path := filepath.Join(root, ".codex", "sessions", "2026", "08", "01", "rollout-2026-08-01T10-00-00-session-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	contents := `{"type":"session_meta","payload":{"id":"session-1","cwd":"/repo/iter-02"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":200000}}}}
`
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, ok, err := LastContextTokens("/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("LastContextTokens: %v", err)
	}
	if ok || got != 0 {
		t.Errorf("LastContextTokens() = (%d, %t), want (0, false)", got, ok)
	}
}

func TestLastContextTokens_MissingOrMalformedSessionDataDoesNotReportContext(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", "")
	root, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	path := filepath.Join(root, ".codex", "sessions", "2026", "08", "01", "rollout-2026-08-01T10-00-00-session-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	contents := "{not json}\n{\"type\":\"session_meta\",\"payload\":{\"id\":\"session-1\",\"cwd\":\"/repo/iter-01\"}}\n"
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, ok, err := LastContextTokens("/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("LastContextTokens: %v", err)
	}
	if ok || got != 0 {
		t.Errorf("LastContextTokens() = (%d, %t), want (0, false)", got, ok)
	}
}

func TestLastRateLimit_ReturnsExhaustedPrimaryQuotaForMatchingSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", "")
	root, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	path := filepath.Join(root, ".codex", "sessions", "2026", "08", "01", "rollout-session-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	contents := `{"type":"session_meta","payload":{"id":"session-1","cwd":"/repo/iter-01"}}
{"type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":100,"resets_at":1786170140},"secondary":{"used_percent":72,"resets_at":1786200000}}}}
`
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, ok, err := LastRateLimit("/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("LastRateLimit: %v", err)
	}
	if !ok {
		t.Fatal("LastRateLimit() ok = false, want true")
	}
	if got.Quota != "primary" || !got.ResetAt.Equal(time.Unix(1786170140, 0)) {
		t.Errorf("LastRateLimit() = %+v, want exhausted primary at %v", got, time.Unix(1786170140, 0))
	}
}

func TestLastRateLimit_LatestQuotaStateClearsAnEarlierExhaustion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", "")
	root, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	path := filepath.Join(root, ".codex", "sessions", "2026", "08", "01", "rollout-session-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	contents := `{"type":"session_meta","payload":{"id":"session-1","cwd":"/repo/iter-01"}}
{"type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":100,"resets_at":1786170140}}}}
{"type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":12,"resets_at":1786170140}}}}
`
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, ok, err := LastRateLimit("/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("LastRateLimit: %v", err)
	}
	if ok {
		t.Error("LastRateLimit() ok = true, want false after quota reset")
	}
}

func TestLastContextTokens_CodexHomeTakesPrecedenceOverDefaultHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)

	path := filepath.Join(codexHome, "sessions", "2026", "08", "01", "rollout-2026-08-01T10-00-00-session-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	contents := `{"type":"session_meta","payload":{"id":"session-1","cwd":"/repo/iter-01"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":151000}}}}
`
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, ok, err := LastContextTokens("/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("LastContextTokens: %v", err)
	}
	if !ok || got != 151000 {
		t.Errorf("LastContextTokens() = (%d, %t), want (151000, true)", got, ok)
	}
}

func TestLastContextTokens_CodexHomeNeverReadsDefaultHomeState(t *testing.T) {
	realHome := t.TempDir()
	t.Setenv("HOME", realHome)
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)

	realPath := filepath.Join(realHome, ".codex", "sessions", "2026", "08", "01", "rollout-2026-08-01T10-00-00-session-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(realPath), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	realContents := `{"type":"session_meta","payload":{"id":"session-1","cwd":"/repo/iter-01"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":999000}}}}
`
	if err := os.WriteFile(realPath, []byte(realContents), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, ok, err := LastContextTokens("/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("LastContextTokens: %v", err)
	}
	if ok || got != 0 {
		t.Errorf("LastContextTokens() = (%d, %t), want (0, false): real HOME state must not be read when CODEX_HOME is set", got, ok)
	}
}

func TestReadStatsAndLastRateLimit_UseSameCodexHomeAsLastContextTokens(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)

	path := filepath.Join(codexHome, "sessions", "2026", "08", "01", "rollout-2026-08-01T10-00-00-session-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	contents := `{"type":"session_meta","timestamp":"2026-08-01T10:00:00Z","payload":{"id":"session-1","cwd":"/repo/iter-01"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":151000},"total_token_usage":{"total_tokens":300000}},"rate_limits":{"primary":{"used_percent":100,"resets_at":1786170140}}}}
`
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	stats, ok, err := ReadStats("/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("ReadStats: %v", err)
	}
	if !ok || stats.TotalTokens != 300000 {
		t.Errorf("ReadStats() = (%+v, %t), want TotalTokens=300000, ok=true", stats, ok)
	}

	limit, ok, err := LastRateLimit("/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("LastRateLimit: %v", err)
	}
	if !ok || limit.Quota != "primary" {
		t.Errorf("LastRateLimit() = (%+v, %t), want exhausted primary, ok=true", limit, ok)
	}
}

func TestLastContextTokens_MissingCodexHomeDirFailsSafe(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "does-not-exist"))

	got, ok, err := LastContextTokens("/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("LastContextTokens: %v", err)
	}
	if ok || got != 0 {
		t.Errorf("LastContextTokens() = (%d, %t), want (0, false)", got, ok)
	}
}
