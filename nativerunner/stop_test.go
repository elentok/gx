package nativerunner_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
)

// movedProcs is the real process table until moved is set; then every pid
// reports another start time, as if the OS reused it.
type movedProcs struct{ moved atomic.Bool }

func (m *movedProcs) Lookup(pid int) (nativerunner.Process, bool, error) {
	proc, ok, err := nativerunner.PS.Lookup(pid)
	if ok && m.moved.Load() {
		proc.Start = "moved"
	}
	return proc, ok, err
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func TestPty_CleanupRefusesWhileHolderLive(t *testing.T) {
	p := newPty(t)
	s, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop(s)

	if err := p.Cleanup("e-01"); err == nil {
		t.Fatal("Cleanup with a live holder = nil, want an error")
	}
	if _, err := os.Stat(filepath.Join(p.Root, "e-01", nativerunner.HolderSocketFile)); err != nil {
		t.Errorf("holder.sock after refused Cleanup: %v", err)
	}
	if _, err := p.Status(s); err != nil {
		t.Errorf("Status after refused Cleanup = %v; Cleanup must not kill", err)
	}
}

func TestPty_StopSkipsMovedPidAndCapsTheWait(t *testing.T) {
	p := newPty(t)
	procs := &movedProcs{}
	p.Procs = procs
	p.StopGrace = 200 * time.Millisecond
	s, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(p.Root, "e-01")
	holderPID := readTestMeta(t, dir).HolderPID
	// A frozen holder without its socket can neither answer Stop nor exit.
	if err := syscall.Kill(holderPID, syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, nativerunner.HolderSocketFile)); err != nil {
		t.Fatal(err)
	}
	procs.moved.Store(true)

	start := time.Now()
	err = p.Stop(s)
	if err == nil || !strings.Contains(err.Error(), fmt.Sprint(holderPID)) {
		t.Fatalf("Stop = %v; want an error naming holder pid %d", err, holderPID)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("Stop took %v; want the wait capped", took)
	}
	if !alive(holderPID) {
		t.Errorf("holder pid %d was signalled though its start time moved", holderPID)
	}
}

func TestHeadless_StopSkipsMovedPidAndCapsTheWait(t *testing.T) {
	h := newHarness(t)
	procs := &movedProcs{}
	h.runner.Procs = procs
	h.runner.StopGrace = 200 * time.Millisecond
	s := h.start(t, "epic-07")
	pid := h.pid(t)
	t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })
	// A frozen claude can't exit on its own, so only a signal would end it.
	if err := syscall.Kill(pid, syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	procs.moved.Store(true)

	start := time.Now()
	err := h.runner.Stop(s)
	if err == nil || !strings.Contains(err.Error(), fmt.Sprint(pid)) {
		t.Fatalf("Stop = %v; want an error naming pid %d", err, pid)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("Stop took %v; want the wait capped", took)
	}
	if !alive(pid) {
		t.Errorf("claude pid %d was signalled though its start time moved", pid)
	}
	if _, ok, _ := h.runner.Find("epic-07"); !ok {
		t.Error("a session that failed to stop was dropped")
	}
}

func readTestMeta(t *testing.T, dir string) nativerunner.Meta {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, nativerunner.MetaFile))
	if err != nil {
		t.Fatal(err)
	}
	var meta nativerunner.Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	return meta
}
