package nativerunner_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/nativerunner"
)

// The test binary doubles as the holder and as a launcher that starts the
// holder detached and exits at once, like the server going away.
const (
	holderEnv       = "GX_TEST_HOLDER"
	holderLaunchEnv = "GX_TEST_HOLDER_LAUNCH"
)

// testHolder takes the gx agent-holder flags: --session-id <id> --dir <dir> -- <cmd...>.
func testHolder() {
	args := os.Args[1:]
	dir := args[slices.Index(args, "--dir")+1]
	if err := nativerunner.RunHolder(dir, args[slices.Index(args, "--")+1:]); err != nil {
		os.Exit(1)
	}
}

func launchHolder() {
	cmd := exec.Command(os.Args[0], os.Args[1:]...)
	cmd.Env = append(os.Environ(), holderEnv+"=1", holderLaunchEnv+"=")
	if err := nativerunner.StartDetached(cmd); err != nil {
		os.Exit(1)
	}
	_, _ = os.Stdout.WriteString(strconv.Itoa(cmd.Process.Pid))
}

func TestHolder_OutlivesLauncherAndRecordsChildExit(t *testing.T) {
	dir := shortTempDir(t)
	stop := filepath.Join(dir, "stop")
	const sessionID = "11111111-2222-4333-8444-555555555555"
	child := `printf 'hello\n'; while [ ! -e "$1" ]; do sleep 0.05; done; printf 'bye\n'; exit 3`

	launcher := exec.Command(os.Args[0], "--session-id", sessionID, "--dir", dir, "--", "sh", "-c", child, "sh", stop)
	launcher.Env = append(os.Environ(), holderLaunchEnv+"=1")
	out, err := launcher.Output()
	if err != nil {
		t.Fatalf("launcher: %v", err)
	}
	pid, err := strconv.Atoi(string(out))
	if err != nil {
		t.Fatalf("launcher printed %q", out)
	}
	t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })

	waitFor(t, "raw.log has the child's output", func() bool {
		raw, _ := os.ReadFile(filepath.Join(dir, nativerunner.RawLogFile))
		return strings.Contains(string(raw), "hello")
	})
	proc, ok, err := nativerunner.PS.Lookup(pid)
	if err != nil || !ok {
		t.Fatalf("holder gone after its launcher exited: ok=%v err=%v", ok, err)
	}
	if !strings.Contains(proc.Cmdline, sessionID) {
		t.Errorf("holder cmdline %q lacks the session id", proc.Cmdline)
	}
	if sid, err := syscall.Getsid(pid); err != nil || sid != pid {
		t.Errorf("holder session = %d, %v; want its own session %d", sid, err, pid)
	}
	if _, err := os.Stat(filepath.Join(dir, nativerunner.ExitCodeFile)); err == nil {
		t.Fatal("exit code written while the child still runs")
	}

	if err := os.WriteFile(stop, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the holder exits", func() bool {
		_, ok, _ := nativerunner.PS.Lookup(pid)
		return !ok
	})
	code, err := os.ReadFile(filepath.Join(dir, nativerunner.ExitCodeFile))
	if err != nil || strings.TrimSpace(string(code)) != "3" {
		t.Errorf("exit code = %q, %v; want 3", code, err)
	}
	scr, err := os.ReadFile(filepath.Join(dir, nativerunner.ScreenFile))
	if err != nil || !strings.Contains(string(scr), "hello") || !strings.Contains(string(scr), "bye") {
		t.Errorf("screen.txt = %q, %v; want the last screen", scr, err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, nativerunner.RawLogFile))
	if !strings.Contains(string(raw), "bye") {
		t.Errorf("raw.log = %q; want the child's last output", raw)
	}
}

func TestHolder_PTYIsWide(t *testing.T) {
	dir := shortTempDir(t)
	if err := nativerunner.RunHolder(dir, []string{"stty", "size"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, nativerunner.RawLogFile))
	if got := strings.Fields(string(raw)); len(got) != 2 || got[1] != strconv.Itoa(nativerunner.HolderCols) {
		t.Errorf("stty size = %q; want %d columns", raw, nativerunner.HolderCols)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting until %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
