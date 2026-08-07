package tickets

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// attachLockFileName is the per-repo attach lock (see loopRegistry's
// attachCount/attachScratchDir): at most one gx process, repo-wide, may be
// Attached to the Queue (actively driving epic runs out of it) at a time.
const attachLockFileName = "queue-attach.json"

type attachLockInfo struct {
	PID       int    `json:"pid"`
	StartTime string `json:"start_time"`
}

func attachLockPath(scratchDir string) string {
	return filepath.Join(scratchDir, attachLockFileName)
}

// processStartTime returns an opaque, comparable marker for pid's start time
// (its ps(1) lstart) and whether pid is currently alive. Comparing two
// captures for the same pid taken at different times distinguishes "still
// the same process" from "pid reused by an unrelated process since a
// reboot" — a bare pid alone can't tell those apart. A package var so tests
// can simulate live/dead/reused pids without spawning real processes.
var processStartTime = func(pid int) (string, bool) {
	out, err := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", false
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return "", false
	}
	return s, true
}

// attachLockIsStale reports whether info's pid is no longer alive, or is
// alive but its current start time no longer matches info's recorded one
// (the pid was reused by a different process).
func attachLockIsStale(info attachLockInfo) bool {
	current, alive := processStartTime(info.PID)
	if !alive {
		return true
	}
	return current != info.StartTime
}

// acquireAttachLock creates scratchDir's attach lock for the current
// process, reclaiming it first if the existing lock is stale (see
// attachLockIsStale). Uses O_EXCL so two processes racing to acquire can't
// both succeed. Returns ok=false with the foreign pid when a different, live
// process already holds the lock.
func acquireAttachLock(scratchDir string) (foreignPID int, ok bool, err error) {
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return 0, false, err
	}
	startTime, alive := processStartTime(os.Getpid())
	if !alive {
		return 0, false, fmt.Errorf("could not determine this process's own start time")
	}
	data, err := json.Marshal(attachLockInfo{PID: os.Getpid(), StartTime: startTime})
	if err != nil {
		return 0, false, err
	}

	path := attachLockPath(scratchDir)
	// One retry covers the reclaim-then-race window: if the lock we just
	// removed for being stale gets recreated by another racing process
	// before our O_EXCL create, we simply lose to it like any other race.
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, writeErr := f.Write(data)
			closeErr := f.Close()
			if writeErr != nil {
				return 0, false, writeErr
			}
			if closeErr != nil {
				return 0, false, closeErr
			}
			return 0, true, nil
		}
		if !os.IsExist(err) {
			return 0, false, err
		}

		existing, readErr := readAttachLock(path)
		if readErr != nil {
			// Unreadable/corrupt lock file: treat it as stale and reclaim.
			_ = os.Remove(path)
			continue
		}
		if !attachLockIsStale(existing) {
			return existing.PID, false, nil
		}
		_ = os.Remove(path)
	}
	return 0, false, fmt.Errorf("could not acquire attach lock at %s", path)
}

func readAttachLock(path string) (attachLockInfo, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return attachLockInfo{}, err
	}
	var info attachLockInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return attachLockInfo{}, err
	}
	return info, nil
}

func releaseAttachLock(scratchDir string) {
	_ = os.Remove(attachLockPath(scratchDir))
}

// attachLockHeld reports whether scratchDir's attach lock is currently held
// by a live process (self or foreign) — false means Detached.
func attachLockHeld(scratchDir string) bool {
	info, err := readAttachLock(attachLockPath(scratchDir))
	if err != nil {
		return false
	}
	return !attachLockIsStale(info)
}
