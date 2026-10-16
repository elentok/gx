package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/tickets"
)

// EventSessionChanged announces a new or changed interactive session; its
// Address is the session id.
const EventSessionChanged = "session-changed"

// Refusal reasons of the interactive-session writes.
const (
	ReasonSessionBadRequest     = "bad-request"
	ReasonSessionUnknownHarness = "unknown-harness"
	ReasonSessionUnknown        = "unknown-session"
	ReasonSessionLive           = "session-live"
	ReasonSessionNotLive        = "session-not-live"
)

// sessionMetaFile holds a session's meta in its state dir.
const sessionMetaFile = "meta.json"

// Session states, as SessionStatus.State.
const (
	SessionStarting    = "starting"
	SessionWorking     = "working"
	SessionIdle        = "idle"
	SessionBlocked     = "blocked"
	SessionRateLimited = "rate-limited"
	SessionExited      = "exited"
)

// How an Exited session ended, as SessionInfo.Exit.
const (
	SessionExitClean   = "clean"
	SessionExitCrashed = "crashed"
)

// holderDialTimeout bounds how long a new session's holder has to open its
// socket before the session counts as crashed.
const holderDialTimeout = 10 * time.Second

// sessionStopTimeout bounds how long a stop waits for the holder to save its
// exit; it outlasts the holder's own SIGTERM-to-SIGKILL grace.
const sessionStopTimeout = 15 * time.Second

// SessionInfo is an interactive session on the wire: everything the Agents
// tab shows.
type SessionInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Project string `json:"project"`
	Dir     string `json:"dir"`
	Harness string `json:"harness"`
	// Ticket is the Session ticket's address.
	Ticket          string        `json:"ticket,omitempty"`
	Status          SessionStatus `json:"status"`
	StatusChangedAt time.Time     `json:"status_changed_at"`
	// Exit is how an Exited session ended.
	Exit string `json:"exit,omitempty"`
	// Stopped marks a session the person stopped; it is never auto-resumed.
	Stopped    bool              `json:"stopped,omitempty"`
	Unread     SessionUnread     `json:"unread"`
	Actionable SessionActionable `json:"actionable"`
}

// SessionStatus is a Session status.
type SessionStatus struct {
	State string `json:"state"`
	// Kind and Message are what a Blocked session waits on.
	Kind       string `json:"kind,omitempty"`
	Message    string `json:"message,omitempty"`
	Background bool   `json:"background,omitempty"`
}

// SessionUnread is a session's Unread; Resumed marks one raised by a resume.
type SessionUnread struct {
	Unread  bool `json:"unread,omitempty"`
	Resumed bool `json:"resumed,omitempty"`
}

// SessionActionable is set while Reason is; Action is its ordered prompts.
type SessionActionable struct {
	Reason string   `json:"reason,omitempty"`
	Action []string `json:"action,omitempty"`
}

// CreateSessionRequest is the body of a session create. Cols and Rows are the
// creating client's terminal panel; zero means the loop default.
type CreateSessionRequest struct {
	Project string `json:"project"`
	// Dir is where the session runs; Worktree replaces it.
	Dir string `json:"dir,omitempty"`
	// Worktree names a new worktree, made from trunk by the Worktrees tab's
	// path and branch rule, for the session to run in. gx never deletes it.
	Worktree string `json:"worktree,omitempty"`
	// Name falls back to Worktree, then the harness's Display name, then
	// `<project>-<n>`.
	Name   string `json:"name,omitempty"`
	Ticket string `json:"ticket,omitempty"`
	// Prompt is typed at the session's first Idle; empty starts it Idle.
	Prompt string `json:"prompt,omitempty"`
	// Harness defaults to claude.
	Harness string `json:"harness,omitempty"`
	Cols    int    `json:"cols,omitempty"`
	Rows    int    `json:"rows,omitempty"`
}

// SessionRequest names the session a stop or remove acts on.
type SessionRequest struct {
	ID string `json:"id"`
}

// SessionResult is a session write's outcome: the session, or a refusal.
type SessionResult struct {
	Session *SessionInfo `json:"session,omitempty"`
	Refused bool         `json:"refused,omitempty"`
	Reason  string       `json:"reason,omitempty"`
	Message string       `json:"message,omitempty"`
}

// sessionMeta is what a session's state dir records about it.
type sessionMeta struct {
	ID      string `json:"id"`
	Harness string `json:"harness"`
	Project string `json:"project"`
	Dir     string `json:"dir"`
	// MadeWorktree marks a Dir gx made for this session: only there is the
	// trust dialog answered for the person.
	MadeWorktree bool   `json:"made_worktree,omitempty"`
	Name         string `json:"name,omitempty"`
	// AutoName marks a fallback name the Display name replaces; a typed name
	// is never replaced.
	AutoName bool   `json:"auto_name,omitempty"`
	Ticket   string `json:"ticket,omitempty"`
	// FirstPrompt waits for the first Idle; it is cleared once typed or on
	// exit.
	FirstPrompt    string    `json:"first_prompt,omitempty"`
	ConversationID string    `json:"conversation_id"`
	CreatedAt      time.Time `json:"created_at"`
	// Cols and Rows are the last size the session's holder started at.
	Cols int `json:"cols,omitempty"`
	Rows int `json:"rows,omitempty"`
	// Status, StatusChangedAt and Exit are the last seen Session status; an
	// empty State is Starting since CreatedAt.
	Status          SessionStatus `json:"status"`
	StatusChangedAt time.Time     `json:"status_changed_at,omitzero"`
	Exit            string        `json:"exit,omitempty"`
	Stopped         bool          `json:"stopped,omitempty"`
	Unread          SessionUnread `json:"unread,omitzero"`
}

func (m sessionMeta) info() SessionInfo {
	info := SessionInfo{
		ID:              m.ID,
		Name:            m.Name,
		Project:         m.Project,
		Dir:             m.Dir,
		Harness:         m.Harness,
		Ticket:          m.Ticket,
		Status:          m.Status,
		StatusChangedAt: m.StatusChangedAt,
		Exit:            m.Exit,
		Stopped:         m.Stopped,
		Unread:          m.Unread,
	}
	if info.Status.State == "" {
		info.Status.State = SessionStarting
	}
	if info.StatusChangedAt.IsZero() {
		info.StatusChangedAt = m.CreatedAt
	}
	return info
}

// sessionStatusOf maps a Watch's status to a Session status.
func sessionStatusOf(st harness.Status) SessionStatus {
	out := SessionStatus{Kind: string(st.Blocked), Message: st.Message, Background: st.Background}
	switch st.State {
	case harness.Working:
		out.State = SessionWorking
	case harness.Idle:
		out.State = SessionIdle
	case harness.Blocked:
		out.State = SessionBlocked
	case harness.RateLimited:
		out.State = SessionRateLimited
	default:
		out.State = SessionStarting
	}
	return out
}

func sessionRefusal(reason, msg string) SessionResult {
	return SessionResult{Refused: true, Reason: reason, Message: msg}
}

func (s *Server) sessionCreateWrite(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.createSession(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

func (s *Server) sessionsRead(w http.ResponseWriter, _ *http.Request) {
	list, err := s.listSessions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, list)
}

// createSession makes the session's state dir, writes its meta and starts its
// holder. It writes no ticket status and takes no slot or permit.
func (s *Server) createSession(req CreateSessionRequest) (SessionResult, error) {
	if req.Project == "" || (req.Dir == "") == (req.Worktree == "") {
		return sessionRefusal(ReasonSessionBadRequest, "a session needs a project and either a dir or a new worktree"), nil
	}
	if req.Worktree == "" {
		if fi, err := os.Stat(req.Dir); err != nil || !fi.IsDir() {
			return sessionRefusal(ReasonSessionBadRequest, fmt.Sprintf("dir %s does not exist", req.Dir)), nil
		}
	}
	if req.Harness == "" {
		req.Harness = string(harness.Claude)
	}
	if _, err := harness.Lookup(agentrunner.Kind(req.Harness)); err != nil {
		return sessionRefusal(ReasonSessionUnknownHarness, err.Error()), nil
	}
	conv, err := nativerunner.NewUUID()
	if err != nil {
		return SessionResult{}, err
	}
	if req.Worktree != "" {
		dir, refusal := s.addSessionWorktree(req.Project, req.Worktree)
		if refusal != "" {
			return sessionRefusal(ReasonSessionBadRequest, refusal), nil
		}
		req.Dir = dir
		if req.Name == "" {
			req.Name = req.Worktree
		}
	}
	meta := sessionMeta{
		Harness:        req.Harness,
		Project:        req.Project,
		Dir:            req.Dir,
		MadeWorktree:   req.Worktree != "",
		Name:           req.Name,
		Ticket:         req.Ticket,
		FirstPrompt:    req.Prompt,
		ConversationID: conv,
		CreatedAt:      time.Now(),
	}
	dir, err := s.saveNewSession(&meta)
	if err != nil {
		return SessionResult{}, err
	}
	id := meta.ID
	if err := s.startSessionHolder(dir, &meta, req.Cols, req.Rows); err != nil {
		return SessionResult{}, err
	}
	s.sessionWatches.start(func() { s.watchSession(dir, id) })
	s.events.publish(EventSessionChanged, id)
	info := meta.info()
	return SessionResult{Session: &info}, nil
}

// addSessionWorktree makes worktree name from the project's trunk, as the
// Worktrees tab does: at <linked worktree dir>/<name> on branch <name>. It
// returns the new worktree's path, or why it could not be made.
func (s *Server) addSessionWorktree(project, name string) (string, string) {
	if !filepath.IsLocal(name) {
		return "", fmt.Sprintf("bad worktree name %q", name)
	}
	projectDir, repoDir, err := s.projectOf(project)
	if err != nil {
		return "", err.Error()
	}
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		return "", err.Error()
	}
	trunk, err := tickets.Trunk(projectDir, repo.Root)
	if err != nil {
		return "", err.Error()
	}
	path := filepath.Join(repo.LinkedWorktreeDir(), name)
	if err := git.AddWorktree(*repo, name, path, trunk); err != nil {
		return "", fmt.Sprintf("create worktree %s: %v", name, err)
	}
	return path, ""
}

// saveNewSession gives meta its id and, when it has no name, the lowest free
// `<project>-<n>`, then saves it. Holding metaMu keeps two creates from
// taking the same name.
func (s *Server) saveNewSession(meta *sessionMeta) (string, error) {
	s.sessionWatches.metaMu.Lock()
	defer s.sessionWatches.metaMu.Unlock()
	if meta.Name == "" {
		list, err := s.listSessions()
		if err != nil {
			return "", err
		}
		meta.Name, meta.AutoName = freeSessionName(meta.Project, list), true
	}
	id, dir, err := s.makeSessionDir()
	if err != nil {
		return "", err
	}
	meta.ID = id
	return dir, writeSessionMeta(dir, *meta)
}

func freeSessionName(project string, list []SessionInfo) string {
	taken := map[string]bool{}
	for _, s := range list {
		taken[s.Name] = true
	}
	for n := 1; ; n++ {
		if name := fmt.Sprintf("%s-%d", project, n); !taken[name] {
			return name
		}
	}
}

// makeSessionDir claims a fresh short id by creating its dir. The id stays
// short because holder.sock lives in the dir and unix socket paths are
// length-limited.
func (s *Server) makeSessionDir() (id, dir string, err error) {
	if err := os.MkdirAll(s.cfg.SessionsDir, 0o700); err != nil {
		return "", "", err
	}
	for range 8 {
		var b [3]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", "", err
		}
		id = hex.EncodeToString(b[:])
		dir = filepath.Join(s.cfg.SessionsDir, id)
		err = os.Mkdir(dir, 0o700)
		if !errors.Is(err, fs.ErrExist) {
			return id, dir, err
		}
	}
	return "", "", errors.New("no free session id")
}

// startSessionHolder runs the harness's Launch in a detached holder, at cols x
// rows, else the session's last saved size, else the loop default, and saves
// meta with the size it used.
func (s *Server) startSessionHolder(dir string, meta *sessionMeta, cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		cols, rows = meta.Cols, meta.Rows
	}
	if cols <= 0 || rows <= 0 {
		cols, rows = nativerunner.HolderCols, nativerunner.HolderRows
	}
	meta.Cols, meta.Rows = cols, rows
	if err := writeSessionMeta(dir, *meta); err != nil {
		return err
	}
	h, err := harness.Lookup(agentrunner.Kind(meta.Harness))
	if err != nil {
		return err
	}
	var extra []string
	if meta.Harness == string(harness.Claude) {
		extra = s.settings.Load().AgentsClaudeArgs
	}
	launch := h.Launch(harness.LaunchOptions{SessionID: meta.ConversationID, Args: extra})
	holder := s.cfg.Holder
	if len(holder) == 0 {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		holder = []string{exe, "agent-holder"}
	}
	args := slices.Concat(holder[1:],
		[]string{
			"--session-id", meta.ConversationID,
			"--harness", meta.Harness,
			"--gx-session", meta.ID,
			"--cols", strconv.Itoa(cols), "--rows", strconv.Itoa(rows),
			"--dir", dir,
		},
		trustFlag(meta.MadeWorktree),
		[]string{"--", launch.Name},
		launch.Args)
	cmd := exec.Command(holder[0], args...)
	cmd.Dir = meta.Dir
	cmd.Env = launch.Env
	if err := nativerunner.StartDetached(cmd); err != nil {
		return fmt.Errorf("start session holder: %w", err)
	}
	// Reap the holder; it outlives the server otherwise.
	go func() { _ = cmd.Wait() }()
	return nil
}

// trustFlag has the holder answer the trust dialog, only for a worktree gx
// made: any other dir is left for the person to trust.
func trustFlag(madeWorktree bool) []string {
	if madeWorktree {
		return []string{"--trust"}
	}
	return nil
}

func writeSessionMeta(dir string, meta sessionMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, sessionMetaFile+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, sessionMetaFile))
}

// listSessions reads every session dir, oldest first. A dir without a
// readable meta is skipped: it is a create still in flight or a torn one.
func (s *Server) listSessions() ([]SessionInfo, error) {
	entries, err := os.ReadDir(s.cfg.SessionsDir)
	if errors.Is(err, fs.ErrNotExist) {
		return []SessionInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	var metas []sessionMeta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		meta, err := readSessionMeta(filepath.Join(s.cfg.SessionsDir, e.Name()))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			s.log.Warn("unreadable session meta", "session", e.Name(), "err", err)
			continue
		}
		metas = append(metas, meta)
	}
	slices.SortFunc(metas, func(a, b sessionMeta) int { return a.CreatedAt.Compare(b.CreatedAt) })
	out := make([]SessionInfo, len(metas))
	for i, m := range metas {
		out[i] = m.info()
	}
	return out, nil
}

func readSessionMeta(dir string) (sessionMeta, error) {
	var meta sessionMeta
	data, err := os.ReadFile(filepath.Join(dir, sessionMetaFile))
	if err != nil {
		return meta, err
	}
	return meta, json.Unmarshal(data, &meta)
}

// sessionWatches runs one watchSession per live session for the server's
// lifetime.
type sessionWatches struct {
	mu      sync.Mutex
	stopped bool
	stop    chan struct{}
	running sync.WaitGroup
	// metaMu serializes the watches' meta read-modify-writes.
	metaMu sync.Mutex
}

func newSessionWatches() *sessionWatches {
	return &sessionWatches{stop: make(chan struct{})}
}

func (w *sessionWatches) start(watch func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	w.running.Add(1)
	go func() {
		defer w.running.Done()
		watch()
	}()
}

func (w *sessionWatches) isStopped() bool {
	select {
	case <-w.stop:
		return true
	default:
		return false
	}
}

// close ends every watch, which records nothing for its stream's end, and
// waits for them.
func (w *sessionWatches) close() {
	w.mu.Lock()
	if !w.stopped {
		w.stopped = true
		close(w.stop)
	}
	w.mu.Unlock()
	w.running.Wait()
}

// watchSession records the session's status and conversation id from its
// holder's watch stream until the stream ends. A stream that ends without an
// exit while the server is up means the holder died: Exited crashed, with no
// retry.
func (s *Server) watchSession(dir, id string) {
	stream, err := s.openSessionWatch(dir)
	if err != nil {
		if !s.sessionWatches.isStopped() {
			s.log.Warn("session watch", "session", id, "err", err)
			s.recordSessionExit(dir, id, savedSessionExit(dir))
		}
		return
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-s.sessionWatches.stop:
		case <-done:
		}
		_ = stream.Close()
	}()
	for {
		msg, err := stream.Next()
		if err != nil {
			if !s.sessionWatches.isStopped() {
				s.recordSessionExit(dir, id, savedSessionExit(dir))
			}
			return
		}
		switch msg.Type {
		case nativerunner.StreamStatus:
			st := sessionStatusOf(msg.Status)
			var prompt string
			s.updateSessionMeta(dir, id, func(m *sessionMeta) bool {
				// A stop records the exit itself, and a status the stream sent
				// before it must not revive the session.
				if m.Status.State == SessionExited {
					return false
				}
				changed := false
				if prev := m.info().Status; prev != st {
					if raisesUnread(prev.State, st.State, msg.Status.Cancelled) {
						s.raiseUnread(m)
					}
					m.Status, m.StatusChangedAt = st, time.Now()
					changed = true
				}
				if msg.ConversationID != "" && msg.ConversationID != m.ConversationID {
					m.ConversationID = msg.ConversationID
					changed = true
				}
				if m.AutoName && msg.DisplayName != "" && msg.DisplayName != m.Name {
					m.Name = msg.DisplayName
					changed = true
				}
				if st.State == SessionIdle && m.FirstPrompt != "" {
					prompt, m.FirstPrompt = m.FirstPrompt, ""
					changed = true
				}
				return changed
			})
			if prompt != "" {
				s.typeFirstPrompt(dir, id, prompt)
			}
		case nativerunner.StreamExit:
			s.recordSessionExit(dir, id, sessionExitOf(msg.ExitCode))
			return
		}
	}
}

// typeFirstPrompt sends Enter on its own, so the harness reads the prompt as
// typed rather than as a paste ending in a newline.
func (s *Server) typeFirstPrompt(dir, id, prompt string) {
	c, err := nativerunner.DialHolder(dir)
	if err == nil {
		err = c.SendKeys(prompt)
	}
	if err == nil {
		err = c.SendKeys("\r")
	}
	if err != nil {
		s.log.Warn("type first prompt", "session", id, "err", err)
	}
}

func sessionExitOf(code int) string {
	if code == 0 {
		return SessionExitClean
	}
	return SessionExitCrashed
}

// savedSessionExit is the exit the holder saved before its socket went, for
// a child that exited before the watch connected; without one the holder
// died.
func savedSessionExit(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, nativerunner.ExitCodeFile))
	if err != nil {
		return SessionExitCrashed
	}
	code, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return SessionExitCrashed
	}
	return sessionExitOf(code)
}

// openSessionWatch waits for a just-started holder's socket, then opens its
// watch stream.
func (s *Server) openSessionWatch(dir string) (*nativerunner.HolderWatch, error) {
	deadline := time.Now().Add(holderDialTimeout)
	for {
		c, err := nativerunner.DialHolder(dir)
		if err == nil {
			if !c.Supports(nativerunner.OpWatch) {
				return nil, errors.New("holder does not serve watch")
			}
			return c.Watch()
		}
		if _, statErr := os.Stat(filepath.Join(dir, nativerunner.ExitCodeFile)); statErr == nil {
			return nil, errors.New("holder exited before its watch opened")
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		select {
		case <-s.sessionWatches.stop:
			return nil, err
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// recordSessionExit records the first exit seen and drops an untyped first
// prompt; a stopped session exits clean whatever the signal left as its code.
func (s *Server) recordSessionExit(dir, id, exit string) {
	s.updateSessionMeta(dir, id, func(m *sessionMeta) bool {
		if m.Status.State == SessionExited {
			return false
		}
		if m.Stopped {
			exit = SessionExitClean
		} else {
			s.raiseUnread(m)
		}
		m.Status, m.StatusChangedAt, m.Exit = SessionStatus{State: SessionExited}, time.Now(), exit
		m.FirstPrompt = ""
		return true
	})
}

// raisesUnread reports whether a move from one live state to another needs
// the person: a finished turn, a block or a rate limit. Idle from Starting is
// the harness coming up, not a turn.
func raisesUnread(from, to string, cancelled bool) bool {
	if from == to {
		return false
	}
	switch to {
	case SessionIdle:
		return !cancelled && from != SessionStarting
	case SessionBlocked, SessionRateLimited:
		return true
	}
	return false
}

// raiseUnread sets m's Unread unless a focused TUI has it on screen.
func (s *Server) raiseUnread(m *sessionMeta) {
	if !s.focus.onScreen(m.ID) {
		m.Unread.Unread = true
	}
}

// sessionWrite adapts a session verb to a POST handler. Refusals are results,
// so they answer 200.
func sessionWrite(do func(SessionRequest) (SessionResult, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req SessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, res)
	}
}

// sessionDir resolves a request's session to its state dir and meta, or the
// refusal for an id that names none.
func (s *Server) sessionDir(id string) (string, sessionMeta, SessionResult, error) {
	if id == "" || filepath.Base(id) != id || strings.HasPrefix(id, ".") {
		return "", sessionMeta{}, sessionRefusal(ReasonSessionBadRequest, fmt.Sprintf("bad session id %q", id)), nil
	}
	dir := filepath.Join(s.cfg.SessionsDir, id)
	meta, err := readSessionMeta(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return "", sessionMeta{}, sessionRefusal(ReasonSessionUnknown, fmt.Sprintf("no session %s", id)), nil
	}
	return dir, meta, SessionResult{}, err
}

// stopSession marks the session stopped by the person, then has its holder
// end claude and waits for the holder to save its exit and last screen. It
// leaves no Unread.
func (s *Server) stopSession(req SessionRequest) (SessionResult, error) {
	dir, res, err := s.markSessionStopped(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	// A holder that is already gone has nothing to stop; its exit is recorded
	// below all the same.
	if c, err := nativerunner.DialHolder(dir); err == nil {
		if err := c.Stop(); err != nil {
			return SessionResult{}, fmt.Errorf("stop session %s: %w", req.ID, err)
		}
		waitForFile(filepath.Join(dir, nativerunner.ExitCodeFile), sessionStopTimeout)
	}
	s.recordSessionExit(dir, req.ID, SessionExitClean)
	meta, err := readSessionMeta(dir)
	if err != nil {
		return SessionResult{}, err
	}
	info := meta.info()
	return SessionResult{Session: &info}, nil
}

// markSessionStopped saves Stopped before the holder is asked to stop, so the
// watch records the exit the stop causes as clean.
func (s *Server) markSessionStopped(id string) (string, SessionResult, error) {
	s.sessionWatches.metaMu.Lock()
	defer s.sessionWatches.metaMu.Unlock()
	dir, meta, res, err := s.sessionDir(id)
	if res.Refused || err != nil {
		return "", res, err
	}
	if meta.Status.State == SessionExited {
		return "", sessionRefusal(ReasonSessionNotLive, fmt.Sprintf("session %s has exited", id)), nil
	}
	meta.Stopped = true
	return dir, SessionResult{}, writeSessionMeta(dir, meta)
}

func waitForFile(path string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// removeSession deletes an Exited session's state dir, never its Dir.
func (s *Server) removeSession(req SessionRequest) (SessionResult, error) {
	s.sessionWatches.metaMu.Lock()
	defer s.sessionWatches.metaMu.Unlock()
	dir, meta, res, err := s.sessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if meta.Status.State != SessionExited {
		return sessionRefusal(ReasonSessionLive, fmt.Sprintf("session %s is live; stop it first", req.ID)), nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return SessionResult{}, err
	}
	s.events.publish(EventSessionChanged, req.ID)
	info := meta.info()
	return SessionResult{Session: &info}, nil
}

// updateSessionMeta applies change to the session's saved meta and, when it
// reports a change, saves it and publishes it.
func (s *Server) updateSessionMeta(dir, id string, change func(*sessionMeta) bool) {
	s.sessionWatches.metaMu.Lock()
	defer s.sessionWatches.metaMu.Unlock()
	meta, err := readSessionMeta(dir)
	if err != nil {
		s.log.Warn("read session meta", "session", id, "err", err)
		return
	}
	if !change(&meta) {
		return
	}
	if err := writeSessionMeta(dir, meta); err != nil {
		s.log.Warn("save session meta", "session", id, "err", err)
		return
	}
	s.events.publish(EventSessionChanged, id)
}
