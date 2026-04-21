package ui

import (
	"strings"
	"testing"
)

func TestCommandOutputLogAppendCommandSanitizesTerminalControlSequences(t *testing.T) {
	log := NewCommandOutputLog()
	log.AppendCommand("git", []string{"push", "origin", "main"}, "\x1b[32mWriting objects: 100%\x1b[0m\r\nremote: \x1b]8;;https://example.com\x07https://example.com\x1b]8;;\x07\n")

	out := log.String()
	if strings.Contains(out, "\x1b") {
		t.Fatalf("expected log to strip ANSI escapes, got %q", out)
	}
	if strings.Contains(out, "\r") {
		t.Fatalf("expected log to strip carriage returns, got %q", out)
	}
	if !strings.Contains(out, "Writing objects: 100%") {
		t.Fatalf("expected sanitized progress text, got %q", out)
	}
	if !strings.Contains(out, "remote: https://example.com") {
		t.Fatalf("expected sanitized hyperlink text, got %q", out)
	}
}

func TestCommandOutputLogFromSanitizesInitialOutput(t *testing.T) {
	log := CommandOutputLogFrom("line 1\r\n\x1b[31mline 2\x1b[0m\r")

	out := log.String()
	if strings.Contains(out, "\x1b") || strings.Contains(out, "\r") {
		t.Fatalf("expected sanitized initial output, got %q", out)
	}
	if out != "line 1\nline 2" {
		t.Fatalf("unexpected sanitized initial output: %q", out)
	}
}
