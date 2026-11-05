package tickets

import (
	"bytes"
	"io"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
)

func TestMain(m *testing.M) { cmdtest.Main(m) }

// testDeps builds Deps over the test's isolated env; a nil writer or getwd
// stays unset.
func testDeps(t *testing.T, stdout, stderr io.Writer, getwd func() (string, error)) Deps {
	t.Helper()
	return Deps{Env: cmdbase.Env{Stdout: stdout, Stderr: stderr, Getwd: getwd, Env: cmdtest.EnvFor(t)}}
}

// execute runs args against `gx tickets` built from d.
func execute(t *testing.T, args []string, d Deps) error {
	t.Helper()
	return cmdtest.Run(t, d.Env, args, New(d))
}

// runIn runs `gx <args>` with cwd as the working directory and returns stdout.
func runIn(t *testing.T, cwd string, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	d := testDeps(t, &stdout, bytes.NewBuffer(nil), func() (string, error) { return cwd, nil })
	err := execute(t, args, d)
	return stdout.String(), err
}
