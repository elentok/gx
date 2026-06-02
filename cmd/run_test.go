package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRunRun_NoArgs(t *testing.T) {
	d := deps{
		stdin:  bytes.NewBuffer(nil),
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
	}
	err := runRun(nil, d)
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("expected usage error, got: %v", err)
	}
}

func TestRunRun_Success(t *testing.T) {
	d := deps{
		stdin:  bytes.NewBuffer(nil),
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
	}
	if err := runRun([]string{"true"}, d); err != nil {
		t.Fatalf("runRun: %v", err)
	}
}

func TestRunRun_Failure_MapsToExitError(t *testing.T) {
	d := deps{
		// Newline so the keep-open wait unblocks instead of hanging.
		stdin:  strings.NewReader("\n"),
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
	}
	err := runRun([]string{"sh", "-c", "exit 5"}, d)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *ExitError, got: %v", err)
	}
	if exitErr.Code != 5 {
		t.Fatalf("exit code = %d, want 5", exitErr.Code)
	}
}

func TestRunRun_ArgsReachChildVerbatim(t *testing.T) {
	var stdout bytes.Buffer
	d := deps{
		stdin:  bytes.NewBuffer(nil),
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
	}
	// Flag-like tokens must pass through untouched (DisableFlagParsing at the
	// cobra layer; here we exercise runRun directly).
	if err := runRun([]string{"printf", "%s", "-m"}, d); err != nil {
		t.Fatalf("runRun: %v", err)
	}
	if got := stdout.String(); got != "-m" {
		t.Fatalf("child stdout = %q, want %q", got, "-m")
	}
}

func TestRunCmd_DisableFlagParsing_PassesFlags(t *testing.T) {
	// Drive through the cobra seam so the `run` subcommand's DisableFlagParsing
	// is exercised: a flag-like token after the program reaches the child.
	var stdout bytes.Buffer
	d := deps{
		stdin:  bytes.NewBuffer(nil),
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return t.TempDir(), nil },
	}
	if err := execute([]string{"run", "printf", "%s", "--force"}, d); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := stdout.String(); got != "--force" {
		t.Fatalf("child stdout = %q, want %q", got, "--force")
	}
}
