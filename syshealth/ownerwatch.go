package syshealth

import (
	"errors"
	"os"
	"strconv"
	"syscall"
	"time"
)

// OwnerPIDEnv names the process a gxfixture server and its holders belong to
// (the test binary that started the fixture). They stop when it is gone, so a
// SIGKILLed owner leaves nothing running. Production never sets it.
const OwnerPIDEnv = "GX_OWNER_PID"

const ownerPoll = time.Second

// WatchOwner polls pid and calls onGone once when it is no longer alive. The
// returned func stops the watch.
func WatchOwner(pid int, poll time.Duration, alive func(int) bool, onGone func()) (stop func()) {
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(poll)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				if !alive(pid) {
					onGone()
					return
				}
			}
		}
	}()
	return func() { close(done) }
}

// WatchOwnerFromEnv starts a watch on the pid in OwnerPIDEnv. It reports
// whether it started one: an unset or malformed value watches nothing.
func WatchOwnerFromEnv(onGone func()) bool {
	pid, err := strconv.Atoi(os.Getenv(OwnerPIDEnv))
	if err != nil || pid <= 0 {
		return false
	}
	WatchOwner(pid, ownerPoll, pidExists, onGone)
	return true
}

// pidExists reports whether a process with pid exists; EPERM means it does,
// under another user.
func pidExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || !errors.Is(err, syscall.ESRCH)
}
