package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_UsesExactlyTheGivenEnv(t *testing.T) {
	dir := t.TempDir()
	trace := filepath.Join(t.TempDir(), "trace2.json")
	env := append(os.Environ(), "GIT_TRACE2_EVENT="+trace)
	if _, _, err := Run(dir, env, "init", "-q"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(trace)
	if err != nil || len(raw) == 0 {
		t.Fatalf("trace file from the given env: %q, %v", raw, err)
	}
}

func TestRunWith_AllowsExitCodesAndFeedsStdin(t *testing.T) {
	dir := t.TempDir()
	env := NonInteractiveEnv()
	if _, _, err := Run(dir, env, "init", "-q"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Run(dir, env, "diff", "--no-index", "--quiet", "/dev/null", "/etc/hosts"); err == nil {
		t.Fatal("exit 1 should be an error without AllowExit")
	}
	if _, _, err := RunWith(dir, env, RunOptions{AllowExit: map[int]bool{1: true}}, "diff", "--no-index", "--quiet", "/dev/null", "/etc/hosts"); err != nil {
		t.Fatalf("allowed exit 1: %v", err)
	}
	out, _, err := RunWith(dir, env, RunOptions{Stdin: strings.NewReader("hello\n")}, "hash-object", "--stdin")
	if err != nil || len(out) != 40 {
		t.Fatalf("hash-object --stdin = %q, %v", out, err)
	}
}

func TestRun_FailureIsARunError(t *testing.T) {
	_, _, err := Run(t.TempDir(), NonInteractiveEnv(), "rev-parse", "--verify", "nope")
	var runErr *RunError
	if !errors.As(err, &runErr) || runErr.Code == 0 {
		t.Fatalf("err = %#v, want *RunError with a code", err)
	}
}

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
