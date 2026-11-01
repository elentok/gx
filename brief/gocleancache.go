package brief

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/elentok/gx/gocachegate"
)

// GoCleanCache waits for the cache gate (no registered build, no `go` process),
// then runs `go clean -cache` in dir. It is not a Tool: it keeps no test log.
// It needs only the state dir, so it works with the server stopped.
func GoCleanCache(dir, stateDir string, env []string, gate gocachegate.Deps, out io.Writer) int {
	exe, err := LookPath(env, "go")
	if err != nil {
		fmt.Fprintf(out, "brief: cannot run go: %v\n", err)
		return ExitCantRun
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		fmt.Fprintf(out, "brief: cannot make the state dir: %v\n", err)
		return ExitCantRun
	}
	release, _, err := gate.WaitExclusive(context.Background(), stateDir)
	if err != nil {
		fmt.Fprintf(out, "brief: cache gate: %v\n", err)
		return ExitCantRun
	}
	defer release()
	cmd := exec.Command(exe, "clean", "-cache")
	cmd.Dir = dir
	cmd.Env = envOrEmpty(env)
	if b, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(out, "brief: go clean -cache failed: %v\n%s", err, b)
		return ExitFail
	}
	fmt.Fprintln(out, "go clean -cache: done")
	return ExitPass
}
