package runner

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_Success_NoFooter(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run("true", nil, IO{In: strings.NewReader(""), Out: &out, Err: &errOut})
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no footer on success, got: %q", errOut.String())
	}
}

func TestRun_Failure_WritesFooterAndReturnsCode(t *testing.T) {
	var out, errOut bytes.Buffer
	// `sh -c 'exit 3'` exits non-zero with a known code.
	code := Run("sh", []string{"-c", "exit 3"}, IO{In: strings.NewReader("\n"), Out: &out, Err: &errOut})
	if code != 3 {
		t.Fatalf("code = %d, want 3", code)
	}
	footer := errOut.String()
	if !strings.Contains(footer, "gx: command failed (exit 3)") {
		t.Fatalf("footer missing failure line: %q", footer)
	}
	if !strings.Contains(footer, "$ sh -c 'exit 3'") {
		t.Fatalf("footer missing quoted command: %q", footer)
	}
	if !strings.Contains(footer, "press Enter to close") {
		t.Fatalf("footer missing dismiss prompt: %q", footer)
	}
}

func TestRun_Failure_UnblocksOnEnter(t *testing.T) {
	// A reader that returns a newline unblocks the wait. If waitForEnter
	// blocked forever this test would hang (and fail by timeout).
	var out, errOut bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- Run("false", nil, IO{In: strings.NewReader("\n"), Out: &out, Err: &errOut})
	}()
	if got := <-done; got != 1 {
		t.Fatalf("code = %d, want 1", got)
	}
}

func TestQuoteCommand(t *testing.T) {
	tests := []struct {
		name    string
		program string
		args    []string
		want    string
	}{
		{name: "plain", program: "git", args: []string{"commit"}, want: "git commit"},
		{name: "spaces", program: "git", args: []string{"commit", "-m", "fix thing"}, want: `git commit -m 'fix thing'`},
		{name: "embedded quote", program: "echo", args: []string{"a'b"}, want: `echo 'a'\''b'`},
		{name: "empty arg", program: "echo", args: []string{""}, want: "echo ''"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := QuoteCommand(tt.program, tt.args); got != tt.want {
				t.Fatalf("QuoteCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}
