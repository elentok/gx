package server

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// serverLock is an flock on a file holding the owner's pid. The kernel drops
// the flock when the process dies, so a crash never leaves a stale lock; the
// pid in the file is only for the "already running" message.
type serverLock struct{ f *os.File }

func acquireLock(path string) (*serverLock, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		defer f.Close()
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, err
		}
		raw, _ := os.ReadFile(path)
		return nil, fmt.Errorf("already running (pid %s)", strings.TrimSpace(string(raw)))
	}
	if err := f.Truncate(0); err != nil {
		f.Close()
		return nil, err
	}
	if _, err := f.WriteAt([]byte(strconv.Itoa(os.Getpid())), 0); err != nil {
		f.Close()
		return nil, err
	}
	return &serverLock{f: f}, nil
}

func (l *serverLock) release() { l.f.Close() }
