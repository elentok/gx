package nativerunner

import (
	"fmt"
	"syscall"
	"time"
)

// signalGroup signals pid's process group only while pid is still the
// process start and sessionID name. The pid may have been reused since it
// was recorded, so a pid that fails the check, or whose lookup errors, is
// never signalled.
func signalGroup(procs ProcessTable, pid int, start, sessionID string, sig syscall.Signal) {
	if live, err := pidAlive(procs, pid, start, sessionID); err == nil && live {
		_ = syscall.Kill(-pid, sig)
	}
}

// awaitExit waits at most grace for exited. A pid signalGroup skipped may
// never exit, so a stop can't block on it forever.
func awaitExit(exited <-chan struct{}, grace time.Duration, pid int) error {
	select {
	case <-exited:
		return nil
	case <-time.After(grace):
		return fmt.Errorf("stop: pid %d did not exit within %v", pid, grace)
	}
}
