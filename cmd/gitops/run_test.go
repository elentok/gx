package gitops

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
)

func runDeps(stdin string, stdout *bytes.Buffer) Deps {
	return Deps{Env: cmdbase.Env{
		Stdin:  strings.NewReader(stdin),
		Stdout: stdout,
		Stderr: bytes.NewBuffer(nil),
	}}
}

func TestRunRun_NoArgs(t *testing.T) {
	t.Parallel()
	err := runRun(nil, runDeps("", bytes.NewBuffer(nil)))
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("expected usage error, got: %v", err)
	}
}

func TestRunRun_Success(t *testing.T) {
	t.Parallel()
	if err := runRun([]string{"true"}, runDeps("", bytes.NewBuffer(nil))); err != nil {
		t.Fatalf("runRun: %v", err)
	}
}

func TestRunRun_Failure_MapsToExitError(t *testing.T) {
	t.Parallel()
	// Newline so the keep-open wait unblocks instead of hanging.
	err := runRun([]string{"sh", "-c", "exit 5"}, runDeps("\n", bytes.NewBuffer(nil)))
	var exitErr *cmdbase.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *ExitError, got: %v", err)
	}
	if exitErr.Code != 5 {
		t.Fatalf("exit code = %d, want 5", exitErr.Code)
	}
}

func TestRunRun_ArgsReachChildVerbatim(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := runDeps("", &stdout)
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
	t.Parallel()
	// Drive through the cobra seam so the `run` subcommand's DisableFlagParsing
	// is exercised: a flag-like token after the program reaches the child.
	var stdout bytes.Buffer
	d := runDeps("", &stdout)
	d.Getwd = func() (string, error) { return t.TempDir(), nil }
	if err := cmdtest.Run(t, d.Env, []string{"run", "printf", "%s", "--force"}, NewRun(d)); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := stdout.String(); got != "--force" {
		t.Fatalf("child stdout = %q, want %q", got, "--force")
	}
}
