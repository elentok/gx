package codexsession

import (
	"os"
	"path/filepath"
	"testing"
)

func writeRollout(t *testing.T, home, sessionID, contents string) {
	t.Helper()
	path := filepath.Join(home, "sessions", "2026", "10", "25", "rollout-2026-10-25T10-00-00-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestLastAssistantTextIn_ReturnsTheLastAssistantMessage(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, home, "s1", `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"text":"first"}]}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"text":"hi"}]}}
{"type":"response_item","payload":{"type":"function_call","role":"assistant"}}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"text":"second"},{"text":"third"}]}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"text":"later"}]}}
`)

	got, ok, err := LastAssistantTextIn(home, "s1")
	if err != nil || !ok || got != "second\n\nthird" {
		t.Errorf("LastAssistantTextIn = (%q, %t, %v), want (\"second\\n\\nthird\", true, nil)", got, ok, err)
	}
}

func TestLastAssistantTextIn_MissingRolloutOrNoAssistantTextIsNotOK(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, home, "s2", `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"text":"hi"}]}}
`)

	for _, id := range []string{"s2", "absent", ""} {
		if got, ok, err := LastAssistantTextIn(home, id); err != nil || ok {
			t.Errorf("LastAssistantTextIn(%q) = (%q, %t, %v), want not ok", id, got, ok, err)
		}
	}
}
