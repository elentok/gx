package nativerunner

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/elentok/gx/agentrunner"
)

// Agent directory layout, shared with reattach.
const (
	MetaFile  = "meta.json"
	StdinFile = "stdin"
	OutFile   = "out.jsonl"
	ErrFile   = "stderr.log"
)

// Meta is what meta.json records about a launched agent.
type Meta struct {
	Runner    string `json:"runner"`
	PID       int    `json:"pid"`
	SessionID string `json:"session_id"`
	Label     string `json:"label"`
	Epic      string `json:"epic"`
	Cwd       string `json:"cwd"`
}

var errUnsupported = errors.New("headless runner: not supported yet")

// Headless runs claude as `claude -p` speaking stream-json, detached in its
// own session so it outlives the server.
type Headless struct {
	// Root holds one agent directory per label.
	Root string
	// Claude is the claude executable; defaults to "claude" on PATH.
	Claude string
	// PollInterval is how often out.jsonl is checked for new events.
	PollInterval time.Duration
	// StopGrace is how long Stop waits after SIGTERM before SIGKILL.
	StopGrace time.Duration
	// InterruptGrace is how long Stop lets a working agent wind down after an
	// interrupt before it sends SIGTERM.
	InterruptGrace time.Duration
	// PromptTimeout is how long Prompt waits for claude to start the turn.
	PromptTimeout time.Duration

	mu       sync.Mutex
	sessions map[string]*headlessSession
}

var _ agentrunner.Runner = (*Headless)(nil)

type headlessSession struct {
	session agentrunner.Session
	epic    string
	cmd     *exec.Cmd
	// stdin is the FIFO's write end. The server is its only writer;
	// writeMu keeps a large prompt and an interrupt from interleaving.
	stdin   *os.File
	writeMu sync.Mutex
	exited  chan struct{}
	tailed  chan struct{}
	// Guarded by Headless.mu. changed is closed and replaced on every
	// update so Wait and Prompt can block on it.
	track   tracker
	changed chan struct{}
	stopped bool
}

func (h *Headless) Start(opts agentrunner.StartOptions) (agentrunner.Session, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.sessions[opts.Label]; ok {
		return agentrunner.Session{}, fmt.Errorf("%w: %s", agentrunner.ErrLabelTaken, opts.Label)
	}

	dir := filepath.Join(h.Root, opts.Label)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return agentrunner.Session{}, err
	}
	sessionID, err := newUUID()
	if err != nil {
		return agentrunner.Session{}, err
	}

	fifo := filepath.Join(dir, StdinFile)
	if err := os.Remove(fifo); err != nil && !errors.Is(err, os.ErrNotExist) {
		return agentrunner.Session{}, err
	}
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		return agentrunner.Session{}, fmt.Errorf("mkfifo: %w", err)
	}
	// Read-write so neither side blocks on open and claude never sees EOF
	// when the server exits.
	stdin, err := os.OpenFile(fifo, os.O_RDWR, 0)
	if err != nil {
		return agentrunner.Session{}, err
	}
	out, err := os.OpenFile(filepath.Join(dir, OutFile), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		stdin.Close()
		return agentrunner.Session{}, err
	}
	defer out.Close()
	stderr, err := os.OpenFile(filepath.Join(dir, ErrFile), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		stdin.Close()
		return agentrunner.Session{}, err
	}
	defer stderr.Close()
	// Only events claude writes from now on belong to this session.
	offset, err := out.Seek(0, io.SeekEnd)
	if err != nil {
		stdin.Close()
		return agentrunner.Session{}, err
	}

	claude := h.Claude
	if claude == "" {
		claude = "claude"
	}
	cmd := exec.Command(claude, launchArgs(sessionID, opts.Args)...)
	cmd.Dir = opts.Cwd
	cmd.Env = launchEnv(os.Environ(), opts.Env)
	cmd.Stdin = stdin
	cmd.Stdout = out
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		stdin.Close()
		return agentrunner.Session{}, err
	}

	ss := &headlessSession{
		session: agentrunner.Session{Label: opts.Label, ID: opts.Label, SessionID: sessionID},
		epic:    opts.Epic,
		cmd:     cmd,
		stdin:   stdin,
		exited:  make(chan struct{}),
		tailed:  make(chan struct{}),
		// Input waits in the FIFO until claude reads it, so the agent can
		// take a prompt as soon as it is launched.
		track:   tracker{sessionID: sessionID},
		changed: make(chan struct{}),
	}
	meta := Meta{Runner: "headless", PID: cmd.Process.Pid, SessionID: sessionID, Label: opts.Label, Epic: opts.Epic, Cwd: opts.Cwd}
	if err := writeMeta(dir, meta); err != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
		stdin.Close()
		return agentrunner.Session{}, err
	}

	go func() {
		_ = cmd.Wait()
		close(ss.exited)
	}()
	go h.tail(ss, filepath.Join(dir, OutFile), offset)

	if h.sessions == nil {
		h.sessions = map[string]*headlessSession{}
	}
	h.sessions[opts.Label] = ss
	return ss.session, nil
}

func launchArgs(sessionID string, extra []string) []string {
	return append([]string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		"--session-id", sessionID,
		"--permission-mode", "auto",
		"--permission-prompt-tool", "stdio",
	}, extra...)
}

// launchEnv drops only the vars that make claude think it is nested inside
// another claude session, which would stop it saving its transcript.
func launchEnv(base, extra []string) []string {
	env := slices.DeleteFunc(slices.Clone(base), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return name == "CLAUDECODE" || name == "CLAUDE_CODE_ENTRYPOINT"
	})
	env = append(env, "DISABLE_AUTOUPDATER=1")
	return append(env, extra...)
}

func writeMeta(dir string, m Meta) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, MetaFile), append(data, '\n'), 0o644)
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

// tail follows out.jsonl from offset and folds each complete line into the
// session's status. Once claude has exited and the file is drained, the
// session is done.
func (h *Headless) tail(ss *headlessSession, path string, offset int64) {
	defer close(ss.tailed)
	f, err := os.Open(path)
	if err != nil {
		h.update(ss, func(t *tracker) { t.exited = true })
		return
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		h.update(ss, func(t *tracker) { t.exited = true })
		return
	}

	interval := h.PollInterval
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var pending []byte
	buf := make([]byte, 64*1024)
	drain := func() {
		for {
			n, err := f.Read(buf)
			pending = append(pending, buf[:n]...)
			if n == 0 || err != nil {
				break
			}
		}
		for {
			i := bytes.IndexByte(pending, '\n')
			if i < 0 {
				return
			}
			line := pending[:i]
			pending = pending[i+1:]
			if e, ok := parseEvent(line); ok {
				h.update(ss, func(t *tracker) { t.apply(e) })
			}
		}
	}
	for {
		select {
		case <-ss.exited:
			drain()
			h.update(ss, func(t *tracker) { t.exited = true })
			return
		case <-ticker.C:
			drain()
		}
	}
}

func (h *Headless) update(ss *headlessSession, fn func(*tracker)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	fn(&ss.track)
	close(ss.changed)
	ss.changed = make(chan struct{})
}

// live returns the session s names; the caller holds h.mu.
func (h *Headless) live(s agentrunner.Session) (*headlessSession, error) {
	ss, ok := h.sessions[s.Label]
	if !ok || ss.session.ID != s.ID {
		return nil, fmt.Errorf("%w: %s", agentrunner.ErrNotFound, s.Label)
	}
	return ss, nil
}

// Prompt writes one user message to claude's stdin and returns once claude
// started a turn for it.
func (h *Headless) Prompt(s agentrunner.Session, text string) error {
	h.mu.Lock()
	ss, err := h.live(s)
	if err != nil {
		h.mu.Unlock()
		return err
	}
	before := ss.track.status()
	h.mu.Unlock()
	switch before.State {
	case agentrunner.StateBlocked:
		return agentrunner.ErrNotReady
	case agentrunner.StateDone:
		return agentrunner.ErrNotDelivered
	}
	uuid, err := newUUID()
	if err != nil {
		return err
	}
	// Claude echoes uuid as command_uuid in its command_lifecycle events.
	err = ss.send(map[string]any{
		"type":    "user",
		"uuid":    uuid,
		"message": map[string]any{"role": "user", "content": text},
	})
	if err != nil {
		return fmt.Errorf("%w: %v", agentrunner.ErrNotDelivered, err)
	}
	return h.awaitTurn(ss, uuid, before.Turn)
}

// awaitTurn waits for claude to start command uuid. A turn that began since
// the prompt also counts: slash commands like /compact may run without a
// command lifecycle.
func (h *Headless) awaitTurn(ss *headlessSession, uuid string, turn int) error {
	timeout := h.PromptTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		h.mu.Lock()
		started := ss.track.started(uuid) || ss.track.turn > turn
		exited, changed := ss.track.exited, ss.changed
		h.mu.Unlock()
		switch {
		case started:
			return nil
		case exited:
			return fmt.Errorf("%w: claude exited", agentrunner.ErrNotDelivered)
		}
		select {
		case <-changed:
		case <-deadline.C:
			return fmt.Errorf("%w: no turn started within %v", agentrunner.ErrNotDelivered, timeout)
		}
	}
}

// send writes one stream-json message to claude's stdin. It runs outside
// Headless.mu: a full FIFO blocks until claude reads, and the tailer must
// keep updating status meanwhile.
func (ss *headlessSession) send(msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	ss.writeMu.Lock()
	defer ss.writeMu.Unlock()
	_, err = ss.stdin.Write(append(data, '\n'))
	return err
}

func (h *Headless) Status(s agentrunner.Session) (agentrunner.Status, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ss, err := h.live(s)
	if err != nil {
		return agentrunner.Status{}, err
	}
	return ss.track.status(), nil
}

func (h *Headless) Wait(s agentrunner.Session, states []agentrunner.State, timeout time.Duration) (agentrunner.Status, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		h.mu.Lock()
		ss, err := h.live(s)
		if err != nil {
			h.mu.Unlock()
			return agentrunner.Status{}, err
		}
		st, changed := ss.track.status(), ss.changed
		h.mu.Unlock()
		if slices.Contains(states, st.State) {
			return st, nil
		}
		select {
		case <-changed:
		case <-deadline.C:
			return st, agentrunner.ErrTimeout
		}
	}
}

// Stop terminates claude's whole process group. A working agent is
// interrupted first and gets InterruptGrace to end its turn, then SIGTERM
// and StopGrace to exit cleanly.
func (h *Headless) Stop(s agentrunner.Session) error {
	h.mu.Lock()
	ss, ok := h.sessions[s.Label]
	if !ok || ss.session.ID != s.ID || ss.stopped {
		h.mu.Unlock()
		return nil
	}
	ss.stopped = true
	working := ss.track.status().State == agentrunner.StateWorking
	h.mu.Unlock()

	if working && ss.interrupt() == nil {
		grace := h.InterruptGrace
		if grace <= 0 {
			grace = 10 * time.Second
		}
		_, _ = h.Wait(s, []agentrunner.State{agentrunner.StateIdle, agentrunner.StateDone}, grace)
	}

	pgid := -ss.cmd.Process.Pid
	_ = syscall.Kill(pgid, syscall.SIGTERM)
	grace := h.StopGrace
	if grace <= 0 {
		grace = 5 * time.Second
	}
	select {
	case <-ss.exited:
	case <-time.After(grace):
		_ = syscall.Kill(pgid, syscall.SIGKILL)
		<-ss.exited
	}
	<-ss.tailed
	ss.stdin.Close()

	h.mu.Lock()
	delete(h.sessions, s.Label)
	h.mu.Unlock()
	return nil
}

func (h *Headless) Find(label string) (agentrunner.Session, bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ss, ok := h.sessions[label]
	if !ok {
		return agentrunner.Session{}, false, nil
	}
	return ss.session, true, nil
}

func (h *Headless) List(epic string) ([]agentrunner.Session, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []agentrunner.Session
	for _, ss := range h.sessions {
		if ss.epic == epic {
			out = append(out, ss.session)
		}
	}
	slices.SortFunc(out, func(a, b agentrunner.Session) int { return strings.Compare(a.Label, b.Label) })
	return out, nil
}

// Interrupt ends claude's current turn through the control protocol; a
// signal would end the whole process, not just the turn.
func (h *Headless) Interrupt(s agentrunner.Session) error {
	h.mu.Lock()
	ss, err := h.live(s)
	h.mu.Unlock()
	if err != nil {
		return err
	}
	return ss.interrupt()
}

func (ss *headlessSession) interrupt() error {
	id, err := newUUID()
	if err != nil {
		return err
	}
	return ss.send(map[string]any{
		"type":       "control_request",
		"request_id": id,
		"request":    map[string]any{"subtype": "interrupt"},
	})
}

func (h *Headless) RateLimit(s agentrunner.Session) (time.Time, bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ss, err := h.live(s)
	if err != nil {
		return time.Time{}, false, err
	}
	return ss.track.resetAt, !ss.track.resetAt.IsZero(), nil
}

func (h *Headless) Answer(agentrunner.Session, agentrunner.Answer) error { return errUnsupported }
