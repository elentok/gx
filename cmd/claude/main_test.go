package claude

import (
	"io"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
)

func TestMain(m *testing.M) { cmdtest.Main(m) }

// testDeps is a Deps writing to stdout and stderr; tests set any other hook on
// the result.
func testDeps(stdout, stderr io.Writer) Deps {
	return Deps{Env: cmdbase.Env{Stdout: stdout, Stderr: stderr}}
}

// payloadDeps is a Deps that reads payload from stdin, writes to out and runs
// in a temp dir outside any git worktree.
func payloadDeps(t *testing.T, out io.Writer, payload string) Deps {
	t.Helper()
	d := testDeps(out, nil)
	d.Stdin = strings.NewReader(payload)
	d.Getwd = func() (string, error) { return t.TempDir(), nil }
	return d
}

// getwdDeps is a Deps whose working directory is dir.
func getwdDeps(dir string) Deps {
	return Deps{Env: cmdbase.Env{Getwd: func() (string, error) { return dir, nil }}}
}
