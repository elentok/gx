package main

import (
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil"
)

func TestShutdownStopsHolderAndChild(t *testing.T) {
	t.Parallel()
	r := start(t, "health")
	meta, ok := readAnyMeta(nativerunner.AgentsRoot(r.StateDir, project))
	if !ok {
		t.Fatal("fixture did not record its held agent")
	}
	owners := []struct {
		role string
		pid  int
	}{
		{"holder", meta.HolderPID},
		{"child", meta.PID},
	}
	// Also clean up on failure so this regression test cannot leak its agent.
	t.Cleanup(func() {
		for _, owner := range owners {
			_ = syscall.Kill(-owner.pid, syscall.SIGKILL)
		}
	})
	r.stop(t)
	for _, owner := range owners {
		if !testutil.Poll(5*time.Second, 50*time.Millisecond, func() bool {
			return syscall.Kill(owner.pid, 0) == syscall.ESRCH
		}) {
			t.Errorf("fixture left %s pid %d running after shutdown", owner.role, owner.pid)
		}
	}
}
