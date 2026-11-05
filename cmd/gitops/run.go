package gitops

import (
	"fmt"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/runner"
)

// runRun is the body of `gx run <program> [args...]`. It runs the child with
// inherited stdio and, on a non-zero exit, keeps the pane open (footer + wait
// for Enter) via the runner module. A non-zero result is mapped to *ExitError
// so main.go forwards the child's exit code (same pass-through as stashify).
func runRun(args []string, d Deps) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: gx run <command> [args...]")
	}

	code := runner.Run(args[0], args[1:], runner.IO{
		In:  d.Stdin,
		Out: d.Stdout,
		Err: d.Stderr,
		Env: d.Env.Env,
	})
	if code != 0 {
		return &cmdbase.ExitError{Code: code}
	}
	return nil
}
