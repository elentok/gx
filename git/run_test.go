package git

import (
	"strings"
	"testing"
)

func TestRunError_Error(t *testing.T) {
	e := &RunError{
		Args:   []string{"status"},
		Dir:    "/repo",
		Stdout: "stdout text",
		Stderr: "stderr text",
		Code:   128,
	}
	msg := e.Error()
	if !strings.Contains(msg, "status") || !strings.Contains(msg, "128") {
		t.Errorf("Error() = %q, expected cmd args and exit code", msg)
	}
}

func TestJoinOutput(t *testing.T) {
	tests := []struct {
		stdout, stderr string
		want           string
	}{
		{"", "", ""},
		{"out", "", "out"},
		{"", "err", "err"},
		{"out", "err", "out\nerr"},
		{"  out  ", "  err  ", "out\nerr"}, // trimmed
	}
	for _, tt := range tests {
		got := joinOutput(tt.stdout, tt.stderr)
		if got != tt.want {
			t.Errorf("joinOutput(%q, %q) = %q, want %q", tt.stdout, tt.stderr, got, tt.want)
		}
	}
}
