package harness

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
	// ClaudeShell is idle with a background task running.
	ClaudeShell   = "shell"
	ClaudeIdle    = "idle"
	ClaudeWaiting = "waiting"
)

// claudePermissionPrompt is the waitingFor of a permission prompt; claude
// waits on a question otherwise.
const claudePermissionPrompt = "permission prompt"

// pidStatusPoll is how often the watcher rereads claude's pid status file.
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

// PidWatcher follows one claude's pid status file and counts its status
// changes as turns. It is the claude harness's Watcher.
type PidWatcher struct {
	path string
	// notBefore is the earliest startedAt, in Unix ms, of a file to trust.
	notBefore int64

	mu   sync.Mutex
	live pidState
	// held, while set, is what state reports in place of the live status.
	held *pidState
}

type pidState struct {
	seen           bool
	last           PidStatus
	conversationID string
	turn           int
}

// pidStartSlack is how far before the spawn claude's startedAt may fall and
// still be this claude's: covers clock rounding, not a reused pid.
const pidStartSlack = 2 * time.Second

// newPidWatcher ignores a file whose startedAt is before spawned: a claude
// that crashed left it, and the OS has since reused its pid.
func newPidWatcher(sessionsDir string, pid int, spawned time.Time) *PidWatcher {
	return &PidWatcher{
		path:      filepath.Join(sessionsDir, strconv.Itoa(pid)+".json"),
		notBefore: spawned.Add(-pidStartSlack).UnixMilli(),
	}
}

// watch polls the file until stop closes.
func (w *PidWatcher) watch(stop <-chan struct{}) {
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
func (w *PidWatcher) poll() {
	data, err := os.ReadFile(w.path)
	if err != nil {
		return
	}
	var f struct {
		PidStatus
		SessionID string `json:"sessionId"`
		StartedAt int64  `json:"startedAt"`
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
	w.live.seen, w.live.last, w.live.conversationID = true, s, f.SessionID
}

// PidState returns the last status read, nil before the file first appears,
// and the turn count.
func (w *PidWatcher) PidState() (*PidStatus, int) {
	st := w.state()
	if !st.seen {
		return nil, st.turn
	}
	return &st.last, st.turn
}

func (w *PidWatcher) state() pidState {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.held != nil {
		return *w.held
	}
	return w.live
}

func (w *PidWatcher) Status() Status {
	st := w.state()
	if !st.seen {
		return Status{State: Starting}
	}
	switch st.last.Status {
	case ClaudeBusy:
		return Status{State: Working}
	case ClaudeShell:
		return Status{State: Working, Background: true}
	case ClaudeIdle:
		return Status{State: Idle}
	case ClaudeWaiting:
		kind := BlockedQuestion
		if st.last.WaitingFor == claudePermissionPrompt {
			kind = BlockedPermission
		}
		return Status{State: Blocked, Blocked: kind, Message: st.last.WaitingFor}
	}
	return Status{State: Starting}
}

func (w *PidWatcher) ConversationID() string {
	return w.state().conversationID
}

// Program ignores OSC 7501 reports: claude's status still comes from its pid
// file alone.
func (w *PidWatcher) Program(ProgramReport) {}

// Hold freezes what PidState and Status report at the current status until
// Release.
func (w *PidWatcher) Hold() {
	w.mu.Lock()
	defer w.mu.Unlock()
	held := w.live
	w.held = &held
}

func (w *PidWatcher) Release() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.held = nil
}

// WaitLive waits up to timeout for the live status to meet cond and returns
// the live turn it met it at.
func (w *PidWatcher) WaitLive(timeout time.Duration, cond func(s PidStatus, turn int) bool) (int, bool) {
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
