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

	mu   sync.Mutex
	seen bool
	last PidStatus
	turn int
}

func newPidWatcher(sessionsDir string, pid int) *pidWatcher {
	return &pidWatcher{path: filepath.Join(sessionsDir, strconv.Itoa(pid)+".json")}
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
	s, err := ParsePidStatus(data)
	if err != nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	// last starts as ClaudeStarting, so a first read past starting still
	// counts the change a poll missed.
	if s.Status != w.last.Status {
		w.turn++
	}
	w.seen, w.last = true, s
}

// state returns the last status read, nil before the file first appears.
func (w *pidWatcher) state() (*PidStatus, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.seen {
		return nil, w.turn
	}
	s := w.last
	return &s, w.turn
}
