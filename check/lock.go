package check

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// The check lock serializes `gx check --full` runs that run the test kind.
//
// Mechanism: a flock(2) on a file in the state dir, not a lease. The kernel
// drops the lock when the holder dies, however it dies, so no stale-lease
// timeout or heartbeat is needed. The holder file and the waiting files are
// only information for the user and the server; the flock decides who runs.
const (
	checkLockName   = "check-full.lock"
	checkHolderName = "check-full.holder"
	checkWaitingDir = "check-waiting"
)

type holder struct {
	PID int    `json:"pid"`
	Run string `json:"run"`
}

// Waiting is the live state of a run that waits for the check lock.
type Waiting struct {
	PID    int    `json:"pid"`
	HeldBy string `json:"held_by"` // run id of the holder, empty when unknown
	Since  string `json:"since"`   // RFC3339
	// Root is the worktree the waiting run checks; the server maps it to the
	// claimed ticket working there.
	Root string `json:"root,omitempty"`
}

// FullLock is a held check lock.
type FullLock struct {
	stateDir string
	f        *os.File
	// Waited is how long acquiring took; zero when the lock was free.
	Waited time.Duration
}

// AcquireFullLock takes the check lock in stateDir, waiting for the holder. While
// it waits it prints a line to wait (nil means none), clears it when done and
// keeps a waiting file in stateDir for readers. root is the worktree the run
// checks, recorded in the waiting file.
func AcquireFullLock(stateDir, root string, wait io.Writer) (*FullLock, error) {
	if err := os.MkdirAll(filepath.Join(stateDir, checkWaitingDir), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(stateDir, checkLockName), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	l := &FullLock{stateDir: stateDir, f: f}
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return l, nil
	}
	if !errors.Is(err, syscall.EWOULDBLOCK) {
		f.Close()
		return nil, err
	}

	held := readHolder(stateDir)
	by := "another run"
	if held.Run != "" {
		by = "run " + held.Run
	}
	if wait != nil {
		fmt.Fprintf(wait, "check: waiting for the check lock held by %s", by)
	}
	wf := filepath.Join(stateDir, checkWaitingDir, strconv.Itoa(os.Getpid())+".json")
	start := time.Now()
	data, _ := json.Marshal(Waiting{PID: os.Getpid(), HeldBy: held.Run, Since: start.UTC().Format(time.RFC3339), Root: root})
	if err := os.WriteFile(wf, data, 0o644); err != nil {
		f.Close()
		return nil, err
	}
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	os.Remove(wf)
	if wait != nil {
		fmt.Fprint(wait, "\r\x1b[K")
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	l.Waited = time.Since(start)
	return l, nil
}

// SetRun records which run holds the lock, for the lines waiters print.
func (l *FullLock) SetRun(run string) {
	data, _ := json.Marshal(holder{PID: os.Getpid(), Run: run})
	_ = os.WriteFile(filepath.Join(l.stateDir, checkHolderName), data, 0o644)
}

// Release gives the lock up.
func (l *FullLock) Release() {
	os.Remove(filepath.Join(l.stateDir, checkHolderName))
	l.f.Close()
}

// readHolder returns the recorded holder, or the zero value when the file is
// missing or names a process that is gone (a stale file of a killed run).
func readHolder(stateDir string) holder {
	var h holder
	data, err := os.ReadFile(filepath.Join(stateDir, checkHolderName))
	if err != nil || json.Unmarshal(data, &h) != nil || !pidAlive(h.PID) {
		return holder{}
	}
	return h
}

// ReadWaiting returns the runs that wait for the check lock right now. A file
// whose process is gone (a killed waiter) is not returned and is removed.
func ReadWaiting(stateDir string) []Waiting {
	dir := filepath.Join(stateDir, checkWaitingDir)
	entries, _ := os.ReadDir(dir)
	var out []Waiting
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		var w Waiting
		data, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(data, &w) != nil {
			continue // unreadable or half written: not accepted, not removed
		}
		if pidAlive(w.PID) {
			out = append(out, w)
		} else {
			os.Remove(path)
		}
	}
	return out
}

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
