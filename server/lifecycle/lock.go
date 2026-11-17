// Package lifecycle owns the server's start, stop and restart plumbing: the
// state-dir lock, the rotating log file, the start-up steps, the restart
// check and the config-reload apply order. It reaches the rest of the server
// through small host interfaces the root implements.
package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// LockFileName is the lock file inside the state dir.
const LockFileName = "server.lock"

// Lock is an flock on a file holding the owner's pid. The kernel drops
// the flock when the process dies, so a crash never leaves a stale lock; the
// pid in the file is only for the "already running" message.
type Lock struct{ f *os.File }

// samePidLockRetry bounds the wait when the lock file names this process. That
// happens when a restart re-locks its state dir while a forked child still
// holds the old fd. The child releases it shortly, so retry instead of failing.
const (
	samePidLockRetry     = 2 * time.Second
	samePidLockRetryTick = 20 * time.Millisecond
	// lockPidReadWait bounds the re-read of a lock file with no full pid yet.
	lockPidReadWait = 500 * time.Millisecond
)

// LockPidWidth is the width of the zero-padded pid in the lock file, wide
// enough for the largest pid any OS allows.
const LockPidWidth = 10

// AcquireLock takes the lock at path or says who holds it.
func AcquireLock(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	deadline := start.Add(samePidLockRetry)
	unreadableDeadline := start.Add(lockPidReadWait)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			f.Close()
			return nil, err
		}
		pid, ok := readLockPid(path)
		switch {
		case !ok:
			// A new file is empty until the winner's first write.
			if !time.Now().Before(unreadableDeadline) {
				f.Close()
				return nil, fmt.Errorf("lock %s is held but holds no pid after %s", path, lockPidReadWait)
			}
		case pid != os.Getpid():
			f.Close()
			return nil, fmt.Errorf("already running (pid %d)", pid)
		case !time.Now().Before(deadline):
			f.Close()
			return nil, fmt.Errorf("lock still held by this process (pid %d) after %s", pid, samePidLockRetry)
		}
		time.Sleep(samePidLockRetryTick)
	}
	// One fixed-width write and no truncate: a shorter pid fully covers an older
	// longer one, so a loser never reads an empty file once a pid was written.
	if _, err := f.WriteAt(fmt.Appendf(nil, "%0*d", LockPidWidth, os.Getpid()), 0); err != nil {
		f.Close()
		return nil, err
	}
	return &Lock{f: f}, nil
}

// readLockPid parses the full-width pid in the lock file. ok is false for an
// empty or short file.
func readLockPid(path string) (pid int, ok bool) {
	raw, _ := os.ReadFile(path)
	if len(raw) != LockPidWidth {
		return 0, false
	}
	pid, err := strconv.Atoi(string(raw))
	return pid, err == nil
}

// Release drops the lock.
func (l *Lock) Release() { l.f.Close() }

// LockHeldBy reports the pid of a live server that holds the lock. Unlike
// AcquireLock it never writes the file, so it is safe to call beside a server.
func LockHeldBy(stateDir string) (int, bool) {
	path := filepath.Join(stateDir, LockFileName)
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		return 0, false
	}
	raw, _ := os.ReadFile(path)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	return pid, true
}
