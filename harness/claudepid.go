package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/elentok/gx/transcript"
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

// PidWatcher follows one claude's pid status file, counting its status
// changes as turns, its OSC 7501 reports and its transcript's rate limits. It
// is the claude harness's Watcher.
type PidWatcher struct {
	path        string
	projectsDir string
	// notBefore is the earliest startedAt, in Unix ms, of a file to trust.
	notBefore int64
	// startup reports a pre-session dialog on screen; nil sees none.
	startup func() bool

	// transcript is read only when its size changes; poll alone touches it.
	transcript struct {
		conversationID, path string
		size                 int64
	}

	mu   sync.Mutex
	live pidState
	// held, while set, is what state reports in place of the live status.
	held *pidState
	// lastLimit is when the newest rate limit applied was written: an older
	// one, still the transcript's last word early in a turn, isn't news.
	lastLimit time.Time
}

type pidState struct {
	seen           bool
	last           PidStatus
	conversationID string
	turn           int
	// program is the last OSC 7501 report, nil before one or after clear.
	program *ProgramReport
	// rateLimit is the transcript's rate limit, held until the next Working.
	rateLimit *transcript.APIError
}

// pidStartSlack is how far before the spawn claude's startedAt may fall and
// still be this claude's: covers clock rounding, not a reused pid.
const pidStartSlack = 2 * time.Second

// newPidWatcher ignores a file whose startedAt is before spawned: a claude
// that crashed left it, and the OS has since reused its pid.
// The transcripts are a sibling of sessionsDir, under the same config dir.
func newPidWatcher(sessionsDir string, pid int, spawned time.Time) *PidWatcher {
	return &PidWatcher{
		path:        filepath.Join(sessionsDir, strconv.Itoa(pid)+".json"),
		projectsDir: filepath.Join(filepath.Dir(sessionsDir), "projects"),
		notBefore:   spawned.Add(-pidStartSlack).UnixMilli(),
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
	// last starts as ClaudeStarting, so a first read past starting still
	// counts the change a poll missed.
	if s.Status != w.live.last.Status {
		w.live.turn++
		if s.Status == ClaudeBusy || s.Status == ClaudeShell {
			w.live.rateLimit = nil
		}
	}
	w.live.seen, w.live.last, w.live.conversationID = true, s, f.SessionID
	w.mu.Unlock()
	w.pollTranscript(f.SessionID)
}

// pollTranscript takes a rate limit written since the last one applied.
func (w *PidWatcher) pollTranscript(conversationID string) {
	t := &w.transcript
	if conversationID != t.conversationID {
		paths, err := transcript.FindByIDIn(w.projectsDir, conversationID)
		if err != nil || len(paths) != 1 {
			return
		}
		t.conversationID, t.path, t.size = conversationID, paths[0], -1
	}
	info, err := os.Stat(t.path)
	if err != nil || info.Size() == t.size {
		return
	}
	t.size = info.Size()
	e, ok, err := transcript.LastAPIError(t.path)
	if err != nil || !ok || !e.RateLimited() || e.Timestamp.UnixMilli() < w.notBefore {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if e.Timestamp.After(w.lastLimit) {
		w.lastLimit, w.live.rateLimit = e.Timestamp, &e
	}
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

// Status puts claude's last OSC 7501 report over its pid file, over a rate
// limit in its transcript, over a pre-session dialog on screen. The rate limit
// still covers their Idle, and a Working from before it: claude ends the
// limited turn as any other, and only the next Working clears the hold.
func (w *PidWatcher) Status() Status {
	st := w.state()
	s, ok := programStatus(st.program)
	// OSC done ends the turn but can't say a background shell outlives it;
	// only the pid file's shell state does.
	if !ok || (st.program.State == "done" && st.seen && st.last.Status == ClaudeShell) {
		s = pidFileStatus(st)
	}
	if st.rateLimit != nil && s.State != Blocked {
		return Status{State: RateLimited, Message: st.rateLimit.Message}
	}
	if s.State == Starting && w.startup != nil && w.startup() {
		return Status{State: Blocked, Blocked: BlockedDialog}
	}
	return s
}

func pidFileStatus(st pidState) Status {
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

// Program keeps the last session-wide report; a per-task one doesn't say
// what the session is doing. clear, sent on exit, hands back to the pid file.
func (w *PidWatcher) Program(r ProgramReport) {
	if r.TaskID != "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	switch r.State {
	case "clear":
		w.live.program = nil
		return
	case "working":
		w.live.rateLimit = nil
	}
	w.live.program = &r
}

func programStatus(r *ProgramReport) (Status, bool) {
	if r == nil {
		return Status{}, false
	}
	switch r.State {
	case "working":
		return Status{State: Working}, true
	case "done":
		return Status{State: Idle}, true
	case "idle":
		return Status{State: Idle, Cancelled: true}, true
	case "blocked":
		kind := BlockedQuestion
		if r.Kind == string(BlockedPermission) {
			kind = BlockedPermission
		}
		return Status{State: Blocked, Blocked: kind, Message: r.Message}, true
	}
	return Status{}, false
}

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
