package nativerunner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Claude pid status file values. No status yet means claude is starting.
const (
	ClaudeStarting = ""
	ClaudeBusy     = "busy"
	ClaudeIdle     = "idle"
	ClaudeWaiting  = "waiting"
)

// pidStatusPoll is how often the holder rereads claude's pid status file.
const pidStatusPoll = 100 * time.Millisecond

// PidStatus is what gx reads from claude's pid status file. WaitingFor is
// set only while Status is waiting.
type PidStatus struct {
	Status     string `json:"status,omitempty"`
	WaitingFor string `json:"waitingFor,omitempty"`
}

// ParsePidStatus parses the contents of claude's pid status file.
func ParsePidStatus(data []byte) (PidStatus, error) {
	var s PidStatus
	err := json.Unmarshal(data, &s)
	return s, err
}

// ClaudeSessionsDir is where claude writes its pid status files: under
// CLAUDE_CONFIG_DIR when set, as claude itself resolves it, else ~/.claude.
func ClaudeSessionsDir() (string, error) {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "sessions"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "sessions"), nil
}

// pidWatcher follows one claude's pid status file and counts its status
// changes as turns.
type pidWatcher struct {
	path string
	// notBefore is the earliest startedAt, in Unix ms, of a file to trust.
	notBefore int64

	mu   sync.Mutex
	live pidState
	// held, while set, is what state reports in place of the live status.
	held *pidState
}

type pidState struct {
	seen bool
	last PidStatus
	turn int
}

// pidStartSlack is how far before the spawn claude's startedAt may fall and
// still be this claude's: covers clock rounding, not a reused pid.
const pidStartSlack = 2 * time.Second

// newPidWatcher ignores a file whose startedAt is before spawned: a claude
// that crashed left it, and the OS has since reused its pid.
func newPidWatcher(sessionsDir string, pid int, spawned time.Time) *pidWatcher {
	return &pidWatcher{
		path:      filepath.Join(sessionsDir, strconv.Itoa(pid)+".json"),
		notBefore: spawned.Add(-pidStartSlack).UnixMilli(),
	}
}

// watch polls the file until stop closes.
func (w *pidWatcher) watch(stop <-chan struct{}) {
	tick := time.NewTicker(pidStatusPoll)
	defer tick.Stop()
	for {
		w.poll()
		select {
		case <-stop:
			return
		case <-tick.C:
		}
	}
}

// poll keeps the last status when the file is gone or half-written: claude
// deletes it on a clean exit and rewrites it in place.
func (w *pidWatcher) poll() {
	data, err := os.ReadFile(w.path)
	if err != nil {
		return
	}
	var f struct {
		PidStatus
		StartedAt int64 `json:"startedAt"`
	}
	if json.Unmarshal(data, &f) != nil || f.StartedAt < w.notBefore {
		return
	}
	s := f.PidStatus
	w.mu.Lock()
	defer w.mu.Unlock()
	// last starts as ClaudeStarting, so a first read past starting still
	// counts the change a poll missed.
	if s.Status != w.live.last.Status {
		w.live.turn++
	}
	w.live.seen, w.live.last = true, s
}

// state returns the last status read, nil before the file first appears.
func (w *pidWatcher) state() (*PidStatus, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	st := w.live
	if w.held != nil {
		st = *w.held
	}
	if !st.seen {
		return nil, st.turn
	}
	return &st.last, st.turn
}

// hold freezes what state reports at the current status until release.
func (w *pidWatcher) hold() {
	w.mu.Lock()
	defer w.mu.Unlock()
	held := w.live
	w.held = &held
}

func (w *pidWatcher) release() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.held = nil
}

// waitLive waits up to timeout for the live status to meet cond and returns
// the live turn it met it at.
func (w *pidWatcher) waitLive(timeout time.Duration, cond func(s PidStatus, turn int) bool) (int, bool) {
	deadline := time.Now().Add(timeout)
	for {
		w.mu.Lock()
		turn, ok := w.live.turn, cond(w.live.last, w.live.turn)
		w.mu.Unlock()
		if ok {
			return turn, true
		}
		if time.Now().After(deadline) {
			return 0, false
		}
		time.Sleep(pidStatusPoll / 4)
	}
}
