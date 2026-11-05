package stagemodal

import (
	"strings"
	"testing"
)

func TestAppendRunningOutputStripsTerminalControlSequences(t *testing.T) {
	var m Model
	m.OpenRunning(80, 24, "Amend", NewRunner(ActionAmend, t.TempDir(), "", ""))

	m.appendRunningOutput("\x1b[32mWriting objects: 100%\x1b[0m\r\n")
	m.appendRunningOutput("remote: \x1b]8;;https://example.com\x07https://example.com\x1b]8;;\x07\n")

	if strings.Contains(m.runningContent, "\x1b") {
		t.Fatalf("expected running content to strip ANSI escapes, got %q", m.runningContent)
	}
	if strings.Contains(m.runningContent, "\r") {
		t.Fatalf("expected running content to strip carriage returns, got %q", m.runningContent)
	}
	if !strings.Contains(m.runningContent, "Writing objects: 100%") {
		t.Fatalf("expected sanitized progress text, got %q", m.runningContent)
	}
	if !strings.Contains(m.runningContent, "remote: https://example.com") {
		t.Fatalf("expected sanitized hyperlink text, got %q", m.runningContent)
	}
}
