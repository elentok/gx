package server

import (
	"cmp"
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
	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/sound"
	"github.com/elentok/gx/tickets"
)

// EventSessionChanged announces a new or changed interactive session; its
// Address is the session id.
const EventSessionChanged = "session-changed"

// Refusal reasons of the interactive-session writes.
const (
	ReasonSessionBadRequest     = "bad-request"
	ReasonSessionUnknownHarness = "unknown-harness"
	ReasonSessionHarnessMissing = "harness-missing"
	ReasonSessionUnknown        = "unknown-session"
	ReasonSessionLive           = "session-live"
	ReasonSessionNotLive        = "session-not-live"
	ReasonSessionNotIdle        = "session-not-idle"
	ReasonSessionNoAction       = "no-action"
	ReasonSessionDirGone        = "dir-gone"
	ReasonSessionNoResume       = "no-resume"
	ReasonSessionNotDesign      = "not-design-session"
	ReasonSessionNameTaken      = "name-taken"
)

// errSessionNameTaken is saveNewSession's refusal of a unique name that a live
// session already has.
var errSessionNameTaken = errors.New("a live session already has this name")

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
	list, err := s.listSessionsWithQueue()
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
	if req.Tier != "" && !req.Tier.Valid() {
		return sessionRefusal(ReasonSessionBadRequest, fmt.Sprintf("unknown tier %q (want light, worker or strongest)", req.Tier)), nil
	}
	from := "the request"
	if req.Harness == "" {
		req.Harness = s.launchHarness(req.Project)
		from = s.defaultHarnessSource(req.Project)
	}
	if _, err := harness.Lookup(agentrunner.Kind(req.Harness)); err != nil {
		return sessionRefusal(ReasonSessionUnknownHarness, err.Error()), nil
	}
	if msg := s.interactiveMissingReason(req.Harness, from); msg != "" {
		return sessionRefusal(ReasonSessionHarnessMissing, msg), nil
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
	tier := cmp.Or(req.Tier, ralphloop.TierWorker)
	if req.launchDesign {
		tier = s.designLaunchTier(req.Ticket)
	}
	meta := sessionMeta{
		Harness:        req.Harness,
		Project:        req.Project,
		Dir:            req.Dir,
		MadeWorktree:   req.Worktree != "" || req.MadeWorktree,
		Name:           req.Name,
		Ticket:         req.Ticket,
		LaunchDesign:   req.launchDesign,
		Design:         req.design || (req.Ticket != "" && s.onMapEpic(req.Ticket)),
		FirstPrompt:    req.Prompt,
		RemoteControl:  req.remoteControl,
		RebaseNote:     req.RebaseNote,
		ConversationID: conv,
		CreatedAt:      time.Now(),
	}
	s.applyTier(&meta, tier)
	dir, err := s.saveNewSession(&meta, req.UniqueName)
	if errors.Is(err, errSessionNameTaken) {
		return sessionRefusal(ReasonSessionNameTaken, fmt.Sprintf("a live session is already named %q", meta.Name)), nil
	}
	if err != nil {
		return SessionResult{}, err
	}
	if meta.Design && meta.Ticket != "" {
		// Cosmetic: a failed stamp must not fail the session.
		if err := s.stampDesignStart(meta.Ticket, meta.CreatedAt); err != nil {
			s.log.Warn("stamp design start", "session", meta.ID, "ticket", meta.Ticket, "err", err)
		}
	}
	id := meta.ID
	s.recordConversation(meta)
	s.sessionWatches.metaMu.Lock()
	err = s.startSessionHolder(dir, &meta, req.Cols, req.Rows, false)
	if err != nil {
		s.endFailedStart(dir, &meta)
	}
	s.sessionWatches.metaMu.Unlock()
	if err != nil {
		return SessionResult{}, err
	}
	s.sessionWatches.start(func() { s.watchSession(dir, id, 0, time.Time{}, false) })
	s.events.publish(EventSessionChanged, id)
	info := meta.info()
	return SessionResult{Session: &info}, nil
}

// applyTier sets the session's tier and the model and effort it resolves to for
// the session's harness and project.
func (s *Server) applyTier(meta *sessionMeta, tier ralphloop.Tier) {
	meta.Tier = tier
	meta.Model, meta.Effort = ralphloop.ResolveTier(s.agentsConfig(meta.Project), ralphloop.AgentFor(meta.Harness), tier)
}

// recordConversation keeps the session's current conversation on the record,
// counted toward its current Session ticket. A session with no ticket records
// nothing.
func (s *Server) recordConversation(m sessionMeta) {
	if m.Ticket == "" || m.ConversationID == "" {
		return
	}
	err := conversations.Save(s.cfg.ConversationsDir, conversations.Record{
		ID: m.ConversationID, Dir: m.Dir, Harness: m.Harness, Ticket: m.Ticket, Design: m.Design, RecordedAt: time.Now(),
	})
	if err != nil {
		s.log.Warn("record conversation", "session", m.ID, "err", err)
	}
}

// saveConversationTotals saves the tokens and cost of the ended conversation
// on its record, so stamps never re-read a transcript that may have aged out.
// An unreadable transcript leaves the record without totals.
func (s *Server) saveConversationTotals(m sessionMeta, conversation string) {
	if m.Ticket == "" || conversation == "" {
		return
	}
	agent := ralphloop.AgentFor(m.Harness)
	home, err := s.envHome()
	if err != nil {
		s.log.Warn("read conversation totals", "session", m.ID, "conversation", conversation, "err", err)
		return
	}
	codexHome, _ := s.codexHome()
	tokens, cost, err := ralphloop.SessionsTotals(ralphloop.SessionHomes{Claude: home, Codex: codexHome}, []ralphloop.SessionRef{{Agent: agent, Cwd: m.Dir, SessionID: conversation}})
	if err != nil {
		s.log.Warn("read conversation totals", "session", m.ID, "conversation", conversation, "err", err)
		return
	}
	if err := conversations.SaveTotals(s.cfg.ConversationsDir, conversation, conversations.Totals{TotalTokens: int(tokens), Cost: cost}); err != nil {
		s.log.Warn("save conversation totals", "session", m.ID, "conversation", conversation, "err", err)
	}
	if m.Design {
		s.saveConversationGrilling(m, conversation)
	}
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
// taking the same name. With unique, a live session that already has the name
// refuses the save with errSessionNameTaken; an Exited one does not.
func (s *Server) saveNewSession(meta *sessionMeta, unique bool) (string, error) {
	s.sessionWatches.metaMu.Lock()
	defer s.sessionWatches.metaMu.Unlock()
	if meta.Name == "" || unique {
		list, err := s.listSessions()
		if err != nil {
			return "", err
		}
		if meta.Name == "" {
			meta.Name, meta.AutoName = freeSessionName(meta.Project, list), true
		} else if slices.ContainsFunc(list, func(si SessionInfo) bool {
			return si.Name == meta.Name && si.Status.State != SessionExited
		}) {
			return "", errSessionNameTaken
		}
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

// startSessionHolder runs the harness's Launch, or its Resume of the saved
// conversation, in a detached holder, at cols x rows, else the session's last
// saved size, else the loop default, and saves meta with the size it used.
// The caller holds metaMu.
func (s *Server) startSessionHolder(dir string, meta *sessionMeta, cols, rows int, resume bool) error {
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
	opts := harness.LaunchOptions{
		SessionID: meta.ConversationID,
		Args:      launchArgs(meta, s.settings.Load().AgentsClaudeArgs, s.codexRoots(meta)),
		Env:       s.cfg.Env,
		Cwd:       meta.Dir,
	}
	launch := h.Launch(opts)
	if resume {
		var ok bool
		if launch, ok = h.Resume(meta.ConversationID, opts); !ok {
			return fmt.Errorf("harness %s cannot resume", meta.Harness)
		}
	}
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

// launchArgs is the extra harness args of a launch or resume. Only Claude and
// Codex get any. The model, effort and remote-control name come from the meta,
// never the settings, so a resume keeps what the launch used.
func launchArgs(meta *sessionMeta, claudeArgs, codexRoots []string) []string {
	switch meta.Harness {
	case string(harness.Codex):
		return harness.CodexArgs(harness.CodexArgsInput{Model: meta.Model, Effort: meta.Effort, WritableRoots: codexRoots})
	case string(harness.Claude):
	default:
		return nil
	}
	args := slices.Clone(claudeArgs)
	if meta.Design {
		args = agentrunner.WithDesignDisallowedTools(args)
	}
	// A configured flag wins over the tier for its knob.
	if meta.Model != "" && !hasFlag(claudeArgs, "--model") {
		args = append(args, "--model", meta.Model)
	}
	if meta.Effort != "" && !hasFlag(claudeArgs, "--effort") {
		args = append(args, "--effort", meta.Effort)
	}
	if meta.RemoteControl != "" {
		args = append(args, "--remote-control", meta.RemoteControl)
	}
	return args
}

// codexRoots are the writable roots of a Codex session: its dir's git dirs and
// the project's ticket-store dir, which holds every epic dir. Other harnesses
// get none.
func (s *Server) codexRoots(meta *sessionMeta) []string {
	if meta.Harness != string(harness.Codex) {
		return nil
	}
	var roots []string
	if gitDir, commonDir, err := git.GitDirs(meta.Dir); err == nil {
		roots = append(roots, gitDir, commonDir)
	}
	if d, err := s.projectDir(meta.Project); err == nil {
		roots = append(roots, d)
	}
	return roots
}

// hasFlag reports whether args hold flag, bare or as flag=value.
func hasFlag(args []string, flag string) bool {
	return slices.ContainsFunc(args, func(a string) bool {
		return a == flag || strings.HasPrefix(a, flag+"=")
	})
}

// endFailedStart saves a session whose holder did not start as Exited
// crashed. Left Starting, it has no holder and no watch: Remove and Resume
// refuse it and the next server start would auto-resume a conversation that
// never ran. The caller holds metaMu.
func (s *Server) endFailedStart(dir string, meta *sessionMeta) {
	s.recordSessionExit(meta, SessionExitCrashed)
	if err := writeSessionMeta(dir, *meta); err != nil {
		s.log.Warn("save session meta", "session", meta.ID, "err", err)
		return
	}
	s.events.publish(EventSessionChanged, meta.ID)
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
		out[i] = s.sessionInfo(m)
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
// retry. Any exit before crashUntil is crashed too. A reconnect attaches to a
// session that may already be mid-turn, so only an idle-to-busy edge it sees
// makes a turn Live; a watch from launch sees every turn start.
func (s *Server) watchSession(dir, id string, run int, crashUntil time.Time, reconnect bool) {
	update := func(change func(*sessionMeta) bool) {
		s.updateSessionMetaOrWarn(dir, id, func(m *sessionMeta) bool { return m.Run == run && change(m) })
	}
	recordExit := func(exit string) {
		if time.Now().Before(crashUntil) {
			exit = SessionExitCrashed
		}
		update(func(m *sessionMeta) bool { return s.recordSessionExit(m, exit) })
	}
	stream, err := s.openSessionWatch(dir)
	if err != nil {
		if !s.sessionWatches.isStopped() {
			s.log.Warn("session watch", "session", id, "err", err)
			recordExit(savedSessionExit(dir))
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
	first := s.newFirstPrompt(dir, id, run)
	defer first.stop()
	chat := s.newChatPoll(dir, id)
	defer chat.close()
	// settle is closed to end the wait of the last idle edge's settleTurn.
	var settle chan struct{}
	endSettle := func() {
		if settle != nil {
			close(settle)
			settle = nil
		}
	}
	defer endSettle()
	seenBusy, sawIdle := false, !reconnect
	for {
		msg, err := stream.Next()
		if err != nil {
			if !s.sessionWatches.isStopped() {
				recordExit(savedSessionExit(dir))
			}
			return
		}
		switch msg.Type {
		case nativerunner.StreamStatus:
			st := sessionStatusOf(msg.Status)
			var prompt, signal string
			var design designRef
			idleEdge := false
			// An idle whose turn start this watch did not see (an idle-to-busy
			// edge) is not Live, e.g. a turn already running at connect.
			live := seenBusy
			// A turn also ends while Working with Background: the agent is
			// waiting for input even though its background task is still alive.
			busy := !st.Promptable()
			if busy {
				endSettle()
			}
			seenBusy = busy && (seenBusy || sawIdle)
			sawIdle = !busy
			update(func(m *sessionMeta) bool {
				// A stop records the exit itself, and a status the stream sent
				// before it must not revive the session.
				if m.Status.State == SessionExited {
					return false
				}
				changed := false
				if prev := m.info().Status; prev != st {
					if raisesUnread(prev.State, st.State, msg.Status.Cancelled) {
						s.raiseUnread(m)
						s.triggerBanner(m.ID, unreadSound(st.State), "")
					}
					m.Status, m.StatusChangedAt = st, time.Now()
					changed = true
					if st.Promptable() {
						m.ReportedStep, m.ReportedTitle = "", ""
					}
					if st.Promptable() && !prev.Promptable() {
						idleEdge = true
						if m.Design {
							design = designRef{m.ConversationID, m.Ticket, live}
						}
					}
				}
				if msg.ConversationID != "" && msg.ConversationID != m.ConversationID {
					s.saveConversationTotals(*m, m.ConversationID)
					m.ConversationID = msg.ConversationID
					s.recordConversation(*m)
					changed = true
				}
				if m.AutoName && msg.DisplayName != "" && msg.DisplayName != m.Name {
					m.Name = msg.DisplayName
					changed = true
				}
				if m.FirstPrompt != "" {
					if sig := first.observe(st.State, msg.Ready); sig != "" {
						prompt, signal, m.FirstPrompt = m.FirstPrompt, sig, ""
						changed = true
					}
				}
				return changed
			})
			first.noteState(st.State)
			s.noteFailure(id)
			chat.setActive(st.State == SessionStarting || st.State == SessionWorking)
			// Marked before the Design check reads the transcript.
			mark, marked := turnMark{}, false
			if idleEdge {
				mark, marked = s.markTurn(dir, id)
			}
			if design.Conversation != "" {
				s.detectDesignActionable(dir, id, design)
			}
			if marked {
				endSettle()
				cancel := make(chan struct{})
				settle = cancel
				s.sessionWatches.start(func() { s.settleTurn(dir, id, mark, design, cancel) })
			}
			if prompt != "" {
				first.start(signal, prompt)
			}
		case nativerunner.StreamInput:
			first.personInput()
			// Typing during a countdown stops only the countdown: the run-end
			// Continue stays raised.
			stopped := s.cancelAutoAdvance(id, "Auto-advance stopped: you typed")
			update(func(m *sessionMeta) bool {
				changed := m.AutoSends != 0
				m.AutoSends = 0
				if stopped || m.Actionable.GxRaised {
					return changed
				}
				changed = changed || m.Actionable.Reason != ""
				m.Actionable = SessionActionable{}
				return changed
			})
		case nativerunner.StreamExit:
			recordExit(sessionExitOf(msg.ExitCode))
			return
		}
	}
}

// typeSessionPrompt sends Enter on its own, so the harness reads the prompt
// as typed rather than as a paste ending in a newline.
func typeSessionPrompt(dir, prompt string) error {
	c, err := nativerunner.DialHolder(dir)
	if err == nil {
		err = c.TypeText(prompt)
	}
	if err == nil {
		err = c.SendKeys("\r")
	}
	return err
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
	c, err := s.dialNewHolder(dir)
	if errors.Is(err, errHolderExited) {
		return nil, errors.New("holder exited before its watch opened")
	}
	if err != nil {
		return nil, err
	}
	if !c.Supports(nativerunner.OpWatch) {
		return nil, errors.New("holder does not serve watch")
	}
	return c.Watch()
}

var errHolderExited = errors.New("holder exited")

// dialNewHolder waits for a just-started holder's socket, until the holder
// saves an exit, holderDialTimeout passes or the server stops.
func (s *Server) dialNewHolder(dir string) (*nativerunner.HolderClient, error) {
	deadline := time.Now().Add(holderDialTimeout)
	for {
		c, err := nativerunner.DialHolder(dir)
		if err == nil {
			return c, nil
		}
		if _, statErr := os.Stat(filepath.Join(dir, nativerunner.ExitCodeFile)); statErr == nil {
			return nil, errHolderExited
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
func (s *Server) recordSessionExit(m *sessionMeta, exit string) bool {
	if m.Status.State == SessionExited {
		return false
	}
	if m.Stopped {
		exit = SessionExitClean
	} else {
		s.raiseUnread(m)
		// A Done Design exiting cleanly is expected: no banner.
		if exit != SessionExitClean || !s.designDone(*m) {
			s.triggerBanner(m.ID, sound.Done, "")
		}
	}
	s.saveConversationTotals(*m, m.ConversationID)
	m.Status, m.StatusChangedAt, m.Exit = SessionStatus{State: SessionExited}, time.Now(), exit
	m.RestoreCrashed = exit == SessionExitCrashed && !m.RestoredAt.IsZero() && time.Since(m.RestoredAt) < restoreCrashWindow
	m.FirstPrompt = ""
	m.Actionable = SessionActionable{}
	return true
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
	return sessionWriteAs(nil, do)
}

func sessionWriteAs(check func(SessionRequest) (SessionResult, error), do func(SessionRequest) (SessionResult, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req SessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if check != nil {
			res, err := check(req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if res.Refused {
				writeJSON(w, res)
				return
			}
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
	dir, starting, res, err := s.markSessionStopped(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	// A Starting session's holder may not have opened its socket yet; left
	// undialed, it would run on with no session to stop it.
	dial := nativerunner.DialHolder
	if starting {
		dial = s.dialNewHolder
	}
	// A holder that is already gone has nothing to stop; its exit is recorded
	// below all the same.
	if c, err := dial(dir); err == nil {
		if err := c.Stop(); err != nil {
			return SessionResult{}, fmt.Errorf("stop session %s: %w", req.ID, err)
		}
		waitForFile(filepath.Join(dir, nativerunner.ExitCodeFile), sessionStopTimeout)
	}
	if err := s.updateSessionMeta(dir, req.ID, func(m *sessionMeta) bool { return s.recordSessionExit(m, SessionExitClean) }); err != nil {
		return SessionResult{}, err
	}
	meta, err := readSessionMeta(dir)
	if err != nil {
		return SessionResult{}, err
	}
	info := meta.info()
	return SessionResult{Session: &info}, nil
}

// markSessionStopped saves Stopped before the holder is asked to stop, so the
// watch records the exit the stop causes as clean. starting reports the
// session was still Starting.
func (s *Server) markSessionStopped(id string) (dir string, starting bool, res SessionResult, err error) {
	s.sessionWatches.metaMu.Lock()
	defer s.sessionWatches.metaMu.Unlock()
	dir, meta, res, err := s.sessionDir(id)
	if res.Refused || err != nil {
		return "", false, res, err
	}
	if meta.Status.State == SessionExited {
		return "", false, sessionRefusal(ReasonSessionNotLive, fmt.Sprintf("session %s has exited", id)), nil
	}
	meta.Stopped = true
	starting = meta.Status.State == "" || meta.Status.State == SessionStarting
	return dir, starting, SessionResult{}, writeSessionMeta(dir, meta)
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
// A Design session this change moved to Exited triggers the design cleanup once
// the lock is released.
func (s *Server) updateSessionMeta(dir, id string, change func(*sessionMeta) bool) error {
	meta, exitedDesign, err := s.saveSessionMetaUpdate(dir, id, change)
	if exitedDesign {
		if project, epic, ok := strings.Cut(epicAddress(meta.Ticket), ":"); ok {
			time.Sleep(s.cfg.SessionExitCleanupDelay)
			s.cleanupDesign(project, epic)
		}
	}
	return err
}

func (s *Server) saveSessionMetaUpdate(dir, id string, change func(*sessionMeta) bool) (meta sessionMeta, exitedDesign bool, err error) {
	s.sessionWatches.metaMu.Lock()
	defer s.sessionWatches.metaMu.Unlock()
	meta, err = readSessionMeta(dir)
	if err != nil {
		return meta, false, fmt.Errorf("read session %s meta: %w", id, err)
	}
	wasExited := meta.Status.State == SessionExited
	if !change(&meta) {
		return meta, false, nil
	}
	if err := writeSessionMeta(dir, meta); err != nil {
		return meta, false, fmt.Errorf("save session %s meta: %w", id, err)
	}
	s.events.publish(EventSessionChanged, id)
	return meta, meta.Design && !wasExited && meta.Status.State == SessionExited, nil
}

// updateSessionMetaOrWarn is updateSessionMeta for background callers, which
// have no request to fail.
func (s *Server) updateSessionMetaOrWarn(dir, id string, change func(*sessionMeta) bool) {
	if err := s.updateSessionMeta(dir, id, change); err != nil {
		s.log.Warn("update session meta", "session", id, "err", err)
	}
}
