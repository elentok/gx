package codexsession

import (
	"os"
	"path/filepath"
	"testing"
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
