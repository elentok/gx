package codexsession

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLastContextTokens_ReturnsLatestMatchingSessionForWorktree(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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

func TestLastContextTokens_IgnoresMissingOrWrongWorktreeSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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
