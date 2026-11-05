package cmd

import (
	"bytes"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
)

// runIn runs `gx <args>` through the full root with cwd as the working
// directory and returns stdout.
func runIn(t *testing.T, cwd string, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	d := Deps{Env: cmdbase.Env{
		Stdout: &stdout,
		Stderr: bytes.NewBuffer(nil),
		Getwd:  func() (string, error) { return cwd, nil },
		Env:    cmdtest.EnvFor(t),
	}}
	err := execute(args, d)
	return stdout.String(), err
}
