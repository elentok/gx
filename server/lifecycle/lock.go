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
)

// AcquireLock takes the lock at path or says who holds it.
func AcquireLock(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(samePidLockRetry)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			f.Close()
			return nil, err
		}
		raw, _ := os.ReadFile(path)
		holder := strings.TrimSpace(string(raw))
		if holder != strconv.Itoa(os.Getpid()) {
			f.Close()
			return nil, fmt.Errorf("already running (pid %s)", holder)
		}
		if !time.Now().Before(deadline) {
			f.Close()
			return nil, fmt.Errorf("lock still held by this process (pid %s) after %s", holder, samePidLockRetry)
		}
		time.Sleep(samePidLockRetryTick)
	}
	if err := f.Truncate(0); err != nil {
		f.Close()
		return nil, err
	}
	if _, err := f.WriteAt([]byte(strconv.Itoa(os.Getpid())), 0); err != nil {
		f.Close()
		return nil, err
	}
	return &Lock{f: f}, nil
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
