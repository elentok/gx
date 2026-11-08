package backgroundtask_test

import (
	"bufio"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/backgroundtask"
)

// start runs script in its own process group, waits for the "ready" line the
// script prints once set up, and reaps it so its end shows.
func start(t *testing.T, script string) (pid int, done <-chan struct{}) {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if line, err := bufio.NewReader(out).ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatalf("ready line = %q, %v", line, err)
	}
	ch := make(chan struct{})
	go func() { _ = cmd.Wait(); close(ch) }()
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
	return cmd.Process.Pid, ch
}

func ended(done <-chan struct{}, within time.Duration) bool {
	select {
	case <-done:
		return true
	case <-time.After(within):
		return false
	}
}

func TestStop_TermEndsAProcessThatHonoursIt(t *testing.T) {
	pid, done := start(t, "echo ready; exec sleep 60")
	if err := backgroundtask.Stop(pid, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if !ended(done, 3*time.Second) {
		t.Fatal("process still running after Stop")
	}
}

// A process that ignores SIGTERM is killed once the grace has passed, and not
// before it.
func TestStop_KillsWhatIgnoresTermAfterTheGrace(t *testing.T) {
	pid, done := start(t, "trap '' TERM; echo ready; while :; do sleep 1; done")
	grace := 600 * time.Millisecond
	began := time.Now()
	if err := backgroundtask.Stop(pid, grace); err != nil {
		t.Fatal(err)
	}
	if !ended(done, 3*time.Second) {
		t.Fatal("process survived SIGKILL")
	}
	if took := time.Since(began); took < grace {
		t.Errorf("Stop returned after %v, before the %v grace", took, grace)
	}
}
