package fixturetest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const tmpPrefix = "gxfb-"

// makeTempDir makes this run's build dir, named gxfb-<pid>-<random>, after
// removing the dirs of runs that died. Run's defer RemoveAll never fires when
// go test kills the binary (timeout, SIGKILL), and each dir holds ~90 MB of
// binaries, so the pid in the name lets the next run tell the orphans apart.
func makeTempDir() (string, error) {
	sweepOrphans(os.TempDir())
	return os.MkdirTemp("", fmt.Sprintf("%s%d-", tmpPrefix, os.Getpid()))
}

// sweepOrphans removes the gxfb-<pid>-* dirs under root whose pid is gone.
func sweepOrphans(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if pid, ok := ownerPid(e.Name()); ok && e.IsDir() && !pidAlive(pid) {
			_ = os.RemoveAll(filepath.Join(root, e.Name()))
		}
	}
}

// ownerPid parses the pid out of a gxfb-<pid>-<random> dir name.
func ownerPid(name string) (int, bool) {
	rest, ok := strings.CutPrefix(name, tmpPrefix)
	if !ok {
		return 0, false
	}
	digits, _, ok := strings.Cut(rest, "-")
	if !ok {
		return 0, false
	}
	pid, err := strconv.Atoi(digits)
	return pid, err == nil && pid > 0
}

// pidAlive reports whether a process with pid exists. EPERM means it exists
// under another user.
func pidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || !errors.Is(err, syscall.ESRCH)
}
