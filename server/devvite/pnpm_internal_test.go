package devvite

import (
	"os/exec"
	"sync"
	"testing"
	"time"
)

type signalLog struct {
	mu    sync.Mutex
	kills []bool
}

func (s *signalLog) record(kill bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kills = append(s.kills, kill)
}

func (s *signalLog) get() []bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]bool(nil), s.kills...)
}

func startExec(t *testing.T, name string, args ...string) (*execProcess, *signalLog, *exec.Cmd) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	proc := newExecProcess(cmd)
	log := &signalLog{}
	proc.signal = log.record
	proc.grace = 50 * time.Millisecond
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return proc, log, cmd
}

func TestExecProcessStop_AlreadyExitedSendsNoSignal(t *testing.T) {
	proc, log, _ := startExec(t, "true")
	_ = proc.Wait()

	proc.Stop()
	<-time.After(3 * proc.grace)

	if got := log.get(); len(got) != 0 {
		t.Fatalf("signals after exit = %v, want none", got)
	}
}

func TestExecProcessStop_ExitBeforeGraceSkipsKill(t *testing.T) {
	proc, log, cmd := startExec(t, "sleep", "30")
	proc.grace = 200 * time.Millisecond

	proc.Stop()
	// The fake signal does not stop the child; end it ourselves inside the grace.
	_ = cmd.Process.Kill()
	_ = proc.Wait()
	<-time.After(2 * proc.grace)

	if got := log.get(); len(got) != 1 || got[0] {
		t.Fatalf("signals = %v, want a single SIGTERM", got)
	}
}
