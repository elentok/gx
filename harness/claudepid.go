package harness

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

// transcriptSearchCap is the longest gap between searches for a transcript
// that isn't there yet.
const transcriptSearchCap = 5 * time.Second

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
	dir, _, err := claudeConfigDirs(nil)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sessions"), nil
}

// ClaudeProjectsDir is where claude writes its transcripts.
func ClaudeProjectsDir() (string, error) { return ClaudeProjectsDirIn(nil) }

// ClaudeProjectsDirIn is ClaudeProjectsDir reading HOME and CLAUDE_CONFIG_DIR
// from env (KEY=VALUE entries, the last one of a key wins). Nil env means the
// process env.
func ClaudeProjectsDirIn(env []string) (string, error) {
	dir, _, err := claudeConfigDirs(env)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "projects"), nil
}

// LookupEnv returns key from env, or from the process env when env is nil.
func LookupEnv(env []string, key string) string {
	if env == nil {
		return os.Getenv(key)
	}
	value := ""
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			value = v
		}
	}
	return value
}

// claudeConfigDirs returns claude's config dir and the dir that holds its
// global .claude.json: the same dir under CLAUDE_CONFIG_DIR, else ~/.claude and
// the home dir.
func claudeConfigDirs(env []string) (configDir, globalDir string, err error) {
	if dir := LookupEnv(env, "CLAUDE_CONFIG_DIR"); dir != "" {
		return dir, dir, nil
	}
	home := LookupEnv(env, "HOME")
	if env == nil {
		if home, err = os.UserHomeDir(); err != nil {
			return "", "", err
		}
	} else if home == "" {
		return "", "", errors.New("HOME is not set in the env")
	}
	return filepath.Join(home, ".claude"), home, nil
}

// PidWatcher follows one claude's pid status file, counting its status
// changes as turns, its OSC 7501 reports and its transcript's rate limits. It
// is the claude harness's Watcher.
type PidWatcher struct {
	// path is the spawned pid's file until a descendant's is found.
	path        string
	pid         int
	sessionsDir string
	projectsDir string
	// notBefore is the earliest startedAt, in Unix ms, of a file to trust.
	notBefore int64
	// startup reports a pre-session dialog on screen; nil sees none.
	startup func() bool
	// inputBox reports claude's prompt input on screen; nil sees none.
	inputBox func() bool
	now      func() time.Time
	find     func(projectsDir, conversationID string) ([]string, error)
	// parents maps each running pid to its parent pid.
	parents func() (map[int]int, error)

	// found latches once a trusted file is read; the search stops there.
	found bool
	// descendant is the search for the file of a claude that a wrapper on
	// PATH runs below the spawned pid. Only poll touches it.
	descendant struct {
		// rejected are pids whose fresh file is not ours.
		rejected   map[int]bool
		searchGap  time.Duration
		nextSearch time.Time
	}

	// transcript is read only when its size changes; poll alone touches it.
	transcript struct {
		conversationID, path string
		size                 int64
		// A session left idle with no prompt never writes its transcript, so
		// the search for it backs off rather than globbing every poll.
		searchID   string
		searchGap  time.Duration
		nextSearch time.Time
	}

	mu   sync.Mutex
	live pidState
	// held, while set, is what state reports in place of the live status.
	held *pidState
	// lastLimit is when the newest rate limit applied was written: an older
	// one, still the transcript's last word early in a turn, isn't news.
	lastLimit time.Time
	onChange  func()
	// readyPidAt and readyOSCAt are when those signals first showed the input
	// ready; ready latches the signal Ready reported first.
	readyPidAt, readyOSCAt time.Time
	ready                  ReadySignal
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
		pid:         pid,
		sessionsDir: sessionsDir,
		projectsDir: filepath.Join(filepath.Dir(sessionsDir), "projects"),
		notBefore:   spawned.Add(-pidStartSlack).UnixMilli(),
		now:         time.Now,
		find:        transcript.FindByIDIn,
		parents:     processParents,
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
	f, ok := w.read(w.path)
	if !ok && !w.found {
		var path string
		if path, f, ok = w.findDescendant(); ok {
			w.path = path
		}
	}
	if !ok {
		return
	}
	w.found = true
	s := f.PidStatus
	w.mu.Lock()
	changed := !w.live.seen || s != w.live.last || f.SessionID != w.live.conversationID
	// last starts as ClaudeStarting, so a first read past starting still
	// counts the change a poll missed.
	// A new sessionId (/clear) is a turn a poll could not otherwise see.
	newSession := w.live.conversationID != "" && f.SessionID != w.live.conversationID
	if s.Status != w.live.last.Status || newSession {
		w.live.turn++
		if s.Status == ClaudeBusy || s.Status == ClaudeShell {
			w.live.rateLimit = nil
		}
	}
	w.live.seen, w.live.last, w.live.conversationID = true, s, f.SessionID
	if s.Status == ClaudeIdle && w.readyPidAt.IsZero() {
		w.readyPidAt = w.now()
	}
	w.mu.Unlock()
	if changed {
		w.changed()
	}
	w.pollTranscript(f.SessionID)
}

type pidFile struct {
	PidStatus
	SessionID string `json:"sessionId"`
	StartedAt int64  `json:"startedAt"`
}

// read reports false for a file that is missing, half-written or older than
// the spawn.
func (w *PidWatcher) read(path string) (pidFile, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return pidFile{}, false
	}
	var f pidFile
	if json.Unmarshal(data, &f) != nil || f.StartedAt < w.notBefore {
		return pidFile{}, false
	}
	return f, true
}

// descendantSearchCap is the longest gap between searches for a descendant's
// pid file.
const descendantSearchCap = time.Second

// findDescendant finds the file of a claude that a wrapper started as a child
// or grandchild of the spawned pid. claude names the file after its own pid,
// so the spawned pid's file never appears. A search reads only files written
// since the spawn and runs ps only when one of them is new; it backs off so a
// session that never writes a file costs little.
func (w *PidWatcher) findDescendant() (string, pidFile, bool) {
	d := &w.descendant
	now := w.now()
	if now.Before(d.nextSearch) {
		return "", pidFile{}, false
	}
	if path, f, ok := w.searchDescendant(); ok {
		return path, f, true
	}
	d.searchGap = min(max(2*d.searchGap, pidStatusPoll), descendantSearchCap)
	d.nextSearch = now.Add(d.searchGap)
	return "", pidFile{}, false
}

func (w *PidWatcher) searchDescendant() (string, pidFile, bool) {
	entries, err := os.ReadDir(w.sessionsDir)
	if err != nil {
		return "", pidFile{}, false
	}
	type candidate struct {
		pid  int
		path string
		f    pidFile
	}
	var candidates []candidate
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".json")
		pid, err := strconv.Atoi(name)
		if !ok || err != nil || pid == w.pid || w.descendant.rejected[pid] {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().UnixMilli() < w.notBefore {
			continue
		}
		path := filepath.Join(w.sessionsDir, e.Name())
		if f, ok := w.read(path); ok {
			candidates = append(candidates, candidate{pid, path, f})
		}
	}
	if len(candidates) == 0 {
		return "", pidFile{}, false
	}
	parents, err := w.parents()
	if err != nil {
		return "", pidFile{}, false
	}
	for _, c := range candidates {
		if descends(parents, c.pid, w.pid) {
			return c.path, c.f, true
		}
		if w.descendant.rejected == nil {
			w.descendant.rejected = map[int]bool{}
		}
		w.descendant.rejected[c.pid] = true
	}
	return "", pidFile{}, false
}

// descends reports whether pid is below ancestor in parents.
func descends(parents map[int]int, pid, ancestor int) bool {
	// The cap stops a cycle a ps snapshot taken mid-fork could show.
	for range 64 {
		ppid, ok := parents[pid]
		if !ok || ppid <= 1 {
			return false
		}
		if ppid == ancestor {
			return true
		}
		pid = ppid
	}
	return false
}

// processParents reads the process table with ps, which works the same on
// macOS and Linux and needs no cgo.
func processParents() (map[int]int, error) {
	out, err := exec.Command("ps", "-A", "-o", "pid=,ppid=").Output()
	if err != nil {
		return nil, err
	}
	parents := map[int]int{}
	for line := range strings.Lines(string(out)) {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 == nil && err2 == nil {
			parents[pid] = ppid
		}
	}
	return parents, nil
}

func (w *PidWatcher) OnChange(f func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onChange = f
}

// changed calls the OnChange func outside the lock, so it may read the
// watcher.
func (w *PidWatcher) changed() {
	w.mu.Lock()
	f := w.onChange
	w.mu.Unlock()
	if f != nil {
		f()
	}
}

// pollTranscript takes a rate limit written since the last one applied.
func (w *PidWatcher) pollTranscript(conversationID string) {
	t := &w.transcript
	if conversationID != t.conversationID {
		now := w.now()
		if conversationID != t.searchID {
			t.searchID, t.searchGap, t.nextSearch = conversationID, pidStatusPoll, now
		}
		if now.Before(t.nextSearch) {
			return
		}
		paths, err := w.find(w.projectsDir, conversationID)
		if err != nil || len(paths) != 1 {
			t.nextSearch = now.Add(t.searchGap)
			t.searchGap = min(2*t.searchGap, transcriptSearchCap)
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
	limited := e.Timestamp.After(w.lastLimit)
	if limited {
		w.lastLimit, w.live.rateLimit = e.Timestamp, &e
	}
	w.mu.Unlock()
	if limited {
		w.changed()
	}
}

var _ PidStatusWatcher = (*PidWatcher)(nil)

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
		w.live.turn++
	}
	w.live.program = &r
	if s, ok := programStatus(&r); ok && s.State == Idle && w.readyOSCAt.IsZero() {
		w.readyOSCAt = w.now()
	}
}

// Ready is the earliest of the pid file's idle, an OSC 7501 idle or done
// report, and the input box on screen. The first two are stamped when seen;
// the screen is read here, so it counts only when neither of them has fired.
func (w *PidWatcher) Ready() ReadySignal {
	if w.Status().State == Blocked {
		return ""
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ready != "" {
		return w.ready
	}
	switch {
	case !w.readyPidAt.IsZero() && (w.readyOSCAt.IsZero() || !w.readyOSCAt.Before(w.readyPidAt)):
		w.ready = ReadyPidFile
	case !w.readyOSCAt.IsZero():
		w.ready = ReadyOSC7501
	case w.inputBox != nil && w.inputBox():
		w.ready = ReadyScreen
	}
	return w.ready
}

// ProgramState is the last session-wide OSC 7501 state, "" before one or
// after clear.
func (w *PidWatcher) ProgramState() string {
	st := w.state()
	if st.program == nil {
		return ""
	}
	return st.program.State
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
	w.held = nil
	w.mu.Unlock()
	w.changed()
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
