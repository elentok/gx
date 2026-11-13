package nativerunner

import (
	"fmt"
	"syscall"
	"time"

	"github.com/elentok/gx/nativerunner/proctable"
)

// signalGroup signals pid's process group only while pid is still the
// process start and sessionID name. The pid may have been reused since it
// was recorded, so a pid that fails the check, or whose lookup errors, is
// never signalled.
func signalGroup(procs proctable.ProcessTable, pid int, start, sessionID string, sig syscall.Signal) {
	if live, err := pidAlive(procs, pid, start, sessionID); err == nil && live {
		_ = syscall.Kill(-pid, sig)
	}
}

// killLeftovers SIGKILLs what claude's session left running after claude
// itself exited (background tasks). A group that no longer has members is not
// signalled, and while one has, its id cannot be reused by another process.
// A task that detached into its own session is out of reach.
func (p *Pty) killLeftovers(ss *ptySession) {
	if ss.claudePID <= 0 {
		return
	}
	if err := syscall.Kill(-ss.claudePID, syscall.SIGKILL); err == nil && p.log != nil {
		p.log.Info("killed leftover background tasks", "session", ss.session.SessionID, "pgid", ss.claudePID)
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
