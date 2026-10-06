package transcript

import (
	"strings"
	"testing"
)

func TestLastAssistantText_ReturnsLastNonSidechainText(t *testing.T) {
	path := writeTranscript(t,
		assistantTextLine("first"),
		assistantTextLine("second"),
		`{"type":"assistant","isSidechain":true,"message":{"content":[{"type":"text","text":"subagent"}]}}`,
		assistantToolUseLine("t1", "Bash"),
	)
	got, ok, err := LastAssistantText(path)
	if err != nil || !ok || got != "second" {
		t.Fatalf("got %q ok=%v err=%v, want %q", got, ok, err, "second")
	}
}

func TestLastAssistantText_TruncatesOnRuneBoundary(t *testing.T) {
	path := writeTranscript(t, assistantTextLine(strings.Repeat("é", MaxAssistantTextBytes)))
	got, ok, err := LastAssistantText(path)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !strings.HasSuffix(got, "…") || len(got) > MaxAssistantTextBytes+len("…") {
		t.Fatalf("not truncated: len=%d", len(got))
	}
	if strings.Contains(got, "�") {
		t.Fatal("cut mid-rune")
	}
}

func TestLastAssistantText_NoAssistantText_NotOK(t *testing.T) {
	path := writeTranscript(t, realUserTurnLine("hi"), assistantToolUseLine("t1", "Bash"))
	if _, ok, err := LastAssistantText(path); ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestLastAssistantText_MissingFile_NotOKNoError(t *testing.T) {
	if _, ok, err := LastAssistantText(t.TempDir() + "/nope.jsonl"); ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}
