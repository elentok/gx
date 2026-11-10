package ghfake

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

var (
	buildOnce sync.Once
	binPath   string
	buildErr  error
)

// Warm builds the gh binary, for a TestMain, so no test's own time pays for the
// cold compile.
func Warm() { build() }

// bin returns the path of the built gh, named gh inside a directory of its own
// so the directory can go on PATH. It is built once per test run.
func bin(t testing.TB) string {
	t.Helper()
	build()
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return binPath
}

func build() {
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "ghfake")
		if err != nil {
			buildErr = err
			return
		}
		binPath = filepath.Join(dir, "gh")
		cmd := exec.Command("go", "build", "-o", binPath, "github.com/elentok/gx/testutil/ghfake/cmd/gh")
		cmd.Env = os.Environ()
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = &buildError{err, string(out)}
		}
	})
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return "build fake gh: " + e.err.Error() + "\n" + e.out }
