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
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
)

// EventSessionChanged announces a new or changed interactive session; its
// Address is the session id.
const EventSessionChanged = "session-changed"

// Refusal reasons of the interactive-session writes.
const (
	ReasonSessionBadRequest     = "bad-request"
	ReasonSessionUnknownHarness = "unknown-harness"
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
	Exit       string            `json:"exit,omitempty"`
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
	Dir     string `json:"dir"`
	Name    string `json:"name,omitempty"`
	Ticket  string `json:"ticket,omitempty"`
	// Harness defaults to claude.
	Harness string `json:"harness,omitempty"`
	Cols    int    `json:"cols,omitempty"`
	Rows    int    `json:"rows,omitempty"`
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
	ID             string    `json:"id"`
	Harness        string    `json:"harness"`
	Project        string    `json:"project"`
	Dir            string    `json:"dir"`
	Name           string    `json:"name,omitempty"`
	Ticket         string    `json:"ticket,omitempty"`
	ConversationID string    `json:"conversation_id"`
	CreatedAt      time.Time `json:"created_at"`
	// Cols and Rows are the last size the session's holder started at.
	Cols int `json:"cols,omitempty"`
	Rows int `json:"rows,omitempty"`
}

func (m sessionMeta) info() SessionInfo {
	return SessionInfo{
		ID:              m.ID,
		Name:            m.Name,
		Project:         m.Project,
		Dir:             m.Dir,
		Harness:         m.Harness,
		Ticket:          m.Ticket,
		Status:          SessionStatus{State: SessionStarting},
		StatusChangedAt: m.CreatedAt,
	}
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
	if req.Project == "" || req.Dir == "" {
		return sessionRefusal(ReasonSessionBadRequest, "a session needs a project and a dir"), nil
	}
	if fi, err := os.Stat(req.Dir); err != nil || !fi.IsDir() {
		return sessionRefusal(ReasonSessionBadRequest, fmt.Sprintf("dir %s does not exist", req.Dir)), nil
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
	id, dir, err := s.makeSessionDir()
	if err != nil {
		return SessionResult{}, err
	}
	meta := sessionMeta{
		ID:             id,
		Harness:        req.Harness,
		Project:        req.Project,
		Dir:            req.Dir,
		Name:           req.Name,
		Ticket:         req.Ticket,
		ConversationID: conv,
		CreatedAt:      time.Now(),
	}
	if err := s.startSessionHolder(dir, &meta, req.Cols, req.Rows); err != nil {
		return SessionResult{}, err
	}
	s.events.publish(EventSessionChanged, id)
	info := meta.info()
	return SessionResult{Session: &info}, nil
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
			"--dir", dir, "--", launch.Name,
		},
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
		data, err := os.ReadFile(filepath.Join(s.cfg.SessionsDir, e.Name(), sessionMetaFile))
		if err != nil {
			continue
		}
		var meta sessionMeta
		if err := json.Unmarshal(data, &meta); err != nil {
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
