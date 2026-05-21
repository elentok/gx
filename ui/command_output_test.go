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

func TestCommandOutputString(t *testing.T) {
	tests := []struct {
		out  CommandOutput
		want string
	}{
		{CommandOutput{Stdout: "out", Stderr: ""}, "out"},
		{CommandOutput{Stdout: "", Stderr: "err"}, "err"},
		{CommandOutput{Stdout: "out", Stderr: "err"}, "out\nerr"},
	}
	for _, tt := range tests {
		if got := tt.out.String(); got != tt.want {
			t.Errorf("CommandOutput.String() = %q, want %q", got, tt.want)
		}
	}
}

func TestCommandOutputRecorder_CapturesOutput(t *testing.T) {
	r := NewCommandOutputRecorder()
	out := r.Output()
	if out.Stdout != "" || out.Stderr != "" {
		t.Errorf("expected empty initial output, got %+v", out)
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

func TestCommandOutputLog_NilString(t *testing.T) {
	var log *CommandOutputLog
	if got := log.String(); got != "" {
		t.Errorf("nil log.String() = %q, want empty", got)
	}
}

func TestCommandOutputLog_AppendNoOutput(t *testing.T) {
	log := NewCommandOutputLog()
	log.AppendCommand("git", []string{"status"}, "  ")
	out := log.String()
	if !strings.Contains(out, "(no output)") {
		t.Errorf("expected '(no output)' for empty output, got %q", out)
	}
}

func TestQuoteCommandOutputArg_Safe(t *testing.T) {
	if got := quoteCommandOutputArg("main"); got != "main" {
		t.Errorf("safe arg = %q, want unquoted", got)
	}
}

func TestQuoteCommandOutputArg_NeedsQuoting(t *testing.T) {
	if got := quoteCommandOutputArg("hello world"); got == "hello world" {
		t.Error("arg with space should be quoted")
	}
}

func TestQuoteCommandOutputArg_Empty(t *testing.T) {
	got := quoteCommandOutputArg("")
	if got == "" {
		t.Error("empty arg should be quoted, not empty")
	}
}
