package backgroundtask

import (
	"errors"
	"syscall"
	"time"
)

// StopGrace is how long a task's process gets after SIGTERM before SIGKILL.
const StopGrace = 2 * time.Second

// Stop ends the process of a task: SIGTERM, then SIGKILL once grace has
// passed and it still lives. Claude writes its own failed notice afterwards,
// so nothing is typed and no exit code is read. The process group goes with a
// wrapper that leads one; a process in claude's own group is signalled alone.
func Stop(pid int, grace time.Duration) error {
	target := pid
	if pgid, err := syscall.Getpgid(pid); err == nil && pgid == pid {
		target = -pid
	}
	if err := syscall.Kill(target, syscall.SIGTERM); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if !alive(pid) {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := syscall.Kill(target, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

// alive is false once pid is gone; its parent (claude) reaps it.
func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
