package holder_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/elentok/gx/nativerunner/holder"
)

// The real mechanism: a holder started with --proc-cap runs its child under
// the limit, so a fork past it fails. The launcher runs the test binary as
// the holder, so the limit never lands on the test process itself.
func TestHolder_ProcCapStopsAForkStorm(t *testing.T) {
	dir := shortTempDir(t)
	out, err := exec.Command("ps", "-u", strconv.Itoa(os.Getuid()), "-o", "pid=").Output()
	if err != nil {
		t.Fatal(err)
	}
	limit := len(strings.Fields(string(out))) + 30
	storm := `i=0; while [ $i -lt 300 ]; do sleep 5 & i=$((i+1)); done; wait`

	launcher := exec.Command(os.Args[0], "--session-id", "11111111-2222-4333-8444-555555555556",
		"--dir", dir, "--proc-cap", fmt.Sprint(limit), "--", "sh", "-c", storm)
	launcher.Env = append(os.Environ(), holder.HolderLaunchEnv+"=1")
	pidOut, err := launcher.Output()
	if err != nil {
		t.Fatalf("launcher: %v", err)
	}
	pid, err := strconv.Atoi(string(pidOut))
	if err != nil {
		t.Fatalf("launcher printed %q", pidOut)
	}
	t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })

	waitFor(t, "a fork fails past the cap", func() bool {
		raw, _ := os.ReadFile(filepath.Join(dir, holder.RawLogFile))
		return strings.Contains(string(raw), "Resource temporarily unavailable")
	})
}
