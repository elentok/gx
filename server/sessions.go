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
	"time"

	"github.com/elentok/gx/server/health"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// errSessionNameTaken is saveNewSession's refusal of a unique name that a live
// session already has.
var errSessionNameTaken = errors.New("a live session already has this name")

// sessionMetaFile holds a session's meta in its state dir.
const sessionMetaFile = "meta.json"

// holderDialTimeout bounds how long a new session's holder has to open its
// socket before the session counts as crashed.
const holderDialTimeout = 10 * time.Second

// sessionStopTimeout bounds how long a stop waits for the holder to save its
// exit; it outlasts the holder's own SIGTERM-to-SIGKILL grace.
const sessionStopTimeout = 15 * time.Second

// sessionStatusOf maps a Watch's status to a Session status.
func sessionStatusOf(st harness.Status) wire.SessionStatus {
	out := wire.SessionStatus{Kind: string(st.Blocked), Message: st.Message, Background: st.Background}
	switch st.State {
	case harness.Working:
		out.State = wire.SessionWorking
	case harness.Idle:
		out.State = wire.SessionIdle
	case harness.Blocked:
		out.State = wire.SessionBlocked
	case harness.RateLimited:
		out.State = wire.SessionRateLimited
	default:
		out.State = wire.SessionStarting
	}
	return out
}

func sessionRefusal(reason, msg string) wire.SessionResult {
	return wire.SessionResult{Refused: true, Reason: reason, Message: msg}
}

func (s *Server) sessionCreateWrite(w http.ResponseWriter, r *http.Request) {
	var req wire.CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.sessions.createSession(req, sessionCtx{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

func (s *Server) sessionsRead(w http.ResponseWriter, _ *http.Request) {
	list, err := s.queue.ListSessionsWithQueue()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, list)
}

// createSession makes the session's state dir, writes its meta and starts its
// holder. It writes no ticket status and takes no slot or permit.
func (mg *Manager) createSession(req wire.CreateSessionRequest, sc sessionCtx) (wire.SessionResult, error) {
	if req.Project == "" || (req.Dir == "") == (req.Worktree == "") {
		return sessionRefusal(wire.ReasonSessionBadRequest, "a session needs a project and either a dir or a new worktree"), nil
	}
	if req.Worktree == "" {
		if fi, err := os.Stat(req.Dir); err != nil || !fi.IsDir() {
			return sessionRefusal(wire.ReasonSessionBadRequest, fmt.Sprintf("dir %s does not exist", req.Dir)), nil
		}
	}
	if req.Tier != "" && !req.Tier.Valid() {
		return sessionRefusal(wire.ReasonSessionBadRequest, fmt.Sprintf("unknown tier %q (want light, worker or strongest)", req.Tier)), nil
	}
	from := "the request"
	if req.Harness == "" {
		req.Harness = mg.srv.projects.LaunchHarness(req.Project)
		from = mg.srv.harness.DefaultHarnessSource(req.Project)
	}
	if _, err := harness.Lookup(agentrunner.Kind(req.Harness)); err != nil {
		return sessionRefusal(wire.ReasonSessionUnknownHarness, err.Error()), nil
	}
	if msg := mg.srv.harness.InteractiveMissingReason(req.Harness, from); msg != "" {
		return sessionRefusal(wire.ReasonSessionHarnessMissing, msg), nil
	}
	conv, err := nativerunner.NewUUID()
	if err != nil {
		return wire.SessionResult{}, err
	}
	if req.Worktree != "" {
		dir, refusal := mg.addSessionWorktree(req.Project, req.Worktree)
		if refusal != "" {
			return sessionRefusal(wire.ReasonSessionBadRequest, refusal), nil
		}
		req.Dir = dir
		if req.Name == "" {
			req.Name = req.Worktree
		}
	}
	tier := cmp.Or(req.Tier, ralphloop.TierWorker)
	if sc.launchDesign {
		tier = mg.srv.designLaunchTier(req.Ticket)
	}
	meta := sessionMeta{
		Harness:        req.Harness,
		Project:        req.Project,
		Dir:            req.Dir,
		MadeWorktree:   req.Worktree != "" || req.MadeWorktree,
		Name:           req.Name,
		Ticket:         req.Ticket,
		LaunchDesign:   sc.launchDesign,
		Design:         sc.design || (req.Ticket != "" && mg.srv.onMapEpic(req.Ticket)),
		FirstPrompt:    req.Prompt,
		RemoteControl:  sc.remoteControl,
		RebaseNote:     req.RebaseNote,
		ConversationID: conv,
		CreatedAt:      time.Now(),
	}
	mg.applyTier(&meta, tier)
	dir, err := mg.saveNewSession(&meta, req.UniqueName)
	if errors.Is(err, errSessionNameTaken) {
		return sessionRefusal(wire.ReasonSessionNameTaken, fmt.Sprintf("a live session is already named %q", meta.Name)), nil
	}
	if err != nil {
		return wire.SessionResult{}, err
	}
	if meta.Design && meta.Ticket != "" {
		// Cosmetic: a failed stamp must not fail the session.
		if err := mg.srv.stampDesignStart(meta.Ticket, meta.CreatedAt); err != nil {
			mg.srv.log.Warn("stamp design start", "session", meta.ID, "ticket", meta.Ticket, "err", err)
		}
	}
	id := meta.ID
	mg.recordConversation(meta)
	mg.watches.metaMu.Lock()
	err = mg.startSessionHolder(dir, &meta, req.Cols, req.Rows, false)
	if err != nil {
		mg.endFailedStart(dir, &meta)
	}
	mg.watches.metaMu.Unlock()
	if err != nil {
		return wire.SessionResult{}, err
	}
	mg.watches.start(func() { mg.watchSession(dir, id, 0, time.Time{}, false) })
	mg.srv.events.Publish(wire.EventSessionChanged, id)
	info := meta.info()
	return wire.SessionResult{Session: &info}, nil
}

// applyTier sets the session's tier and the model and effort it resolves to for
// the session's harness and project.
func (mg *Manager) applyTier(meta *sessionMeta, tier ralphloop.Tier) {
	meta.Tier = tier
	meta.Model, meta.Effort = ralphloop.ResolveTier(mg.srv.projects.AgentsConfig(meta.Project), ralphloop.AgentFor(meta.Harness), tier)
}

// recordConversation keeps the session's current conversation on the record,
// counted toward its current Session ticket. A session with no ticket records
// nothing.
func (mg *Manager) recordConversation(m sessionMeta) {
	if m.Ticket == "" || m.ConversationID == "" {
		return
	}
	err := conversations.Save(mg.srv.cfg.ConversationsDir, conversations.Record{
		ID: m.ConversationID, Dir: m.Dir, Harness: m.Harness, Ticket: m.Ticket, Design: m.Design, RecordedAt: time.Now(),
	})
	if err != nil {
		mg.srv.log.Warn("record conversation", "session", m.ID, "err", err)
	}
}

// saveConversationTotals saves the tokens and cost of the ended conversation
// on its record, so stamps never re-read a transcript that may have aged out.
// An unreadable transcript leaves the record without totals.
func (mg *Manager) saveConversationTotals(m sessionMeta, conversation string) {
	if m.Ticket == "" || conversation == "" {
		return
	}
	agent := ralphloop.AgentFor(m.Harness)
	home, err := mg.srv.projects.EnvHome()
	if err != nil {
		mg.srv.log.Warn("read conversation totals", "session", m.ID, "conversation", conversation, "err", err)
		return
	}
	codexHome, _ := mg.srv.projects.CodexHome()
	tokens, cost, err := ralphloop.SessionsTotals(ralphloop.SessionHomes{Claude: home, Codex: codexHome}, []ralphloop.SessionRef{{Agent: agent, Cwd: m.Dir, SessionID: conversation}})
	if err != nil {
		mg.srv.log.Warn("read conversation totals", "session", m.ID, "conversation", conversation, "err", err)
		return
	}
	if err := conversations.SaveTotals(mg.srv.cfg.ConversationsDir, conversation, conversations.Totals{TotalTokens: int(tokens), Cost: cost}); err != nil {
		mg.srv.log.Warn("save conversation totals", "session", m.ID, "conversation", conversation, "err", err)
	}
	if m.Design {
		mg.saveConversationGrilling(m, conversation)
	}
}

// addSessionWorktree makes worktree name from the project's trunk, as the
// Worktrees tab does: at <linked worktree dir>/<name> on branch <name>. It
// returns the new worktree's path, or why it could not be made.
func (mg *Manager) addSessionWorktree(project, name string) (string, string) {
	if !filepath.IsLocal(name) {
		return "", fmt.Sprintf("bad worktree name %q", name)
	}
	projectDir, repoDir, err := mg.srv.projects.Of(project)
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
func (mg *Manager) saveNewSession(meta *sessionMeta, unique bool) (string, error) {
	mg.watches.metaMu.Lock()
	defer mg.watches.metaMu.Unlock()
	if meta.Name == "" || unique {
		list, err := mg.ListSessions()
		if err != nil {
			return "", err
		}
		if meta.Name == "" {
			meta.Name, meta.AutoName = freeSessionName(meta.Project, list), true
		} else if slices.ContainsFunc(list, func(si wire.SessionInfo) bool {
			return si.Name == meta.Name && si.Status.State != wire.SessionExited
		}) {
			return "", errSessionNameTaken
		}
	}
	id, dir, err := mg.makeSessionDir()
	if err != nil {
		return "", err
	}
	meta.ID = id
	return dir, writeSessionMeta(dir, *meta)
}

func freeSessionName(project string, list []wire.SessionInfo) string {
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
func (mg *Manager) makeSessionDir() (id, dir string, err error) {
	if err := os.MkdirAll(mg.srv.cfg.SessionsDir, 0o700); err != nil {
		return "", "", err
	}
	for range 8 {
		var b [3]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", "", err
		}
		id = hex.EncodeToString(b[:])
		dir = filepath.Join(mg.srv.cfg.SessionsDir, id)
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
func (mg *Manager) startSessionHolder(dir string, meta *sessionMeta, cols, rows int, resume bool) error {
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
	env := mg.srv.cfg.Env
	if meta.Harness == string(harness.Codex) {
		// The SessionStart hook writes Codex's own session id here; without it
		// the meta keeps gx's UUID and the chat misses the rollout.
		env = append(slices.Clone(env), harness.CodexSessionEnv(dir)...)
	}
	opts := harness.LaunchOptions{
		SessionID: meta.ConversationID,
		Args:      launchArgs(meta, mg.srv.settings.Load().AgentsClaudeArgs, mg.codexRoots(meta)),
		Env:       env,
		Cwd:       meta.Dir,
	}
	launch := h.Launch(opts)
	if resume {
		var ok bool
		if launch, ok = h.Resume(meta.ConversationID, opts); !ok {
			return fmt.Errorf("harness %s cannot resume", meta.Harness)
		}
	}
	holder := mg.srv.cfg.Holder
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
func (mg *Manager) codexRoots(meta *sessionMeta) []string {
	if meta.Harness != string(harness.Codex) {
		return nil
	}
	var extra []string
	if d, err := mg.srv.projects.Dir(meta.Project); err == nil {
		extra = append(extra, d)
	}
	return harness.CodexWritableRoots(meta.Dir, extra...)
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
func (mg *Manager) endFailedStart(dir string, meta *sessionMeta) {
	mg.recordSessionExit(meta, wire.SessionExitCrashed)
	if err := writeSessionMeta(dir, *meta); err != nil {
		mg.srv.log.Warn("save session meta", "session", meta.ID, "err", err)
		return
	}
	mg.srv.events.Publish(wire.EventSessionChanged, meta.ID)
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

// ListSessions reads every session dir, oldest first. A dir without a
// readable meta is skipped: it is a create still in flight or a torn one.
func (mg *Manager) ListSessions() ([]wire.SessionInfo, error) {
	metas, err := mg.listSessionMetas()
	if err != nil {
		return nil, err
	}
	out := make([]wire.SessionInfo, len(metas))
	for i, m := range metas {
		out[i] = mg.srv.sessionInfo(m)
	}
	return out, nil
}

// listSessionMetas is listSessions before the info is built.
func (mg *Manager) listSessionMetas() ([]sessionMeta, error) {
	entries, err := os.ReadDir(mg.srv.cfg.SessionsDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var metas []sessionMeta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		meta, err := readSessionMeta(filepath.Join(mg.srv.cfg.SessionsDir, e.Name()))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			mg.srv.log.Warn("unreadable session meta", "session", e.Name(), "err", err)
			continue
		}
		metas = append(metas, meta)
	}
	slices.SortFunc(metas, func(a, b sessionMeta) int { return a.CreatedAt.Compare(b.CreatedAt) })
	return metas, nil
}

func readSessionMeta(dir string) (sessionMeta, error) {
	var meta sessionMeta
	data, err := os.ReadFile(filepath.Join(dir, sessionMetaFile))
	if err != nil {
		return meta, err
	}
	return meta, json.Unmarshal(data, &meta)
}

// sessionMetas gives the health owner probe its session reads until sessions
// move out of the root.
type sessionMetas struct{}

func (sessionMetas) Meta(dir string) (health.SessionMeta, error) {
	meta, err := readSessionMeta(dir)
	return health.SessionMeta{Run: meta.Run, Name: meta.Name, Ticket: meta.Ticket}, err
}

// SessionDir resolves a request's session to its state dir and meta, or the
// refusal for an id that names none.
func (mg *Manager) SessionDir(id string) (string, sessionMeta, wire.SessionResult, error) {
	if id == "" || filepath.Base(id) != id || strings.HasPrefix(id, ".") {
		return "", sessionMeta{}, sessionRefusal(wire.ReasonSessionBadRequest, fmt.Sprintf("bad session id %q", id)), nil
	}
	dir := filepath.Join(mg.srv.cfg.SessionsDir, id)
	meta, err := readSessionMeta(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return "", sessionMeta{}, sessionRefusal(wire.ReasonSessionUnknown, fmt.Sprintf("no session %s", id)), nil
	}
	return dir, meta, wire.SessionResult{}, err
}

// stopSession marks the session stopped by the person, then has its holder
// end claude and waits for the holder to save its exit and last screen. It
// leaves no Unread.
func (mg *Manager) stopSession(req wire.SessionRequest) (wire.SessionResult, error) {
	dir, starting, res, err := mg.markSessionStopped(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	// A Starting session's holder may not have opened its socket yet; left
	// undialed, it would run on with no session to stop it.
	dial := nativerunner.DialHolder
	if starting {
		dial = mg.dialNewHolder
	}
	// A holder that is already gone has nothing to stop; its exit is recorded
	// below all the same.
	if c, err := dial(dir); err == nil {
		if err := c.Stop(); err != nil {
			return wire.SessionResult{}, fmt.Errorf("stop session %s: %w", req.ID, err)
		}
		waitForFile(filepath.Join(dir, nativerunner.ExitCodeFile), sessionStopTimeout)
	}
	if err := mg.updateSessionMeta(dir, req.ID, func(m *sessionMeta) bool { return mg.recordSessionExit(m, wire.SessionExitClean) }); err != nil {
		return wire.SessionResult{}, err
	}
	meta, err := readSessionMeta(dir)
	if err != nil {
		return wire.SessionResult{}, err
	}
	info := meta.info()
	return wire.SessionResult{Session: &info}, nil
}

// markSessionStopped saves Stopped before the holder is asked to stop, so the
// watch records the exit the stop causes as clean. starting reports the
// session was still Starting.
func (mg *Manager) markSessionStopped(id string) (dir string, starting bool, res wire.SessionResult, err error) {
	mg.watches.metaMu.Lock()
	defer mg.watches.metaMu.Unlock()
	dir, meta, res, err := mg.SessionDir(id)
	if res.Refused || err != nil {
		return "", false, res, err
	}
	if meta.Status.State == wire.SessionExited {
		return "", false, sessionRefusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s has exited", id)), nil
	}
	meta.Stopped = true
	starting = meta.Status.State == "" || meta.Status.State == wire.SessionStarting
	return dir, starting, wire.SessionResult{}, writeSessionMeta(dir, meta)
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
func (mg *Manager) removeSession(req wire.SessionRequest) (wire.SessionResult, error) {
	mg.watches.metaMu.Lock()
	defer mg.watches.metaMu.Unlock()
	dir, meta, res, err := mg.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if meta.Status.State != wire.SessionExited {
		return sessionRefusal(wire.ReasonSessionLive, fmt.Sprintf("session %s is live; stop it first", req.ID)), nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return wire.SessionResult{}, err
	}
	mg.srv.events.Publish(wire.EventSessionChanged, req.ID)
	info := meta.info()
	return wire.SessionResult{Session: &info}, nil
}

// updateSessionMeta applies change to the session's saved meta and, when it
// reports a change, saves it and publishes it.
// A Design session this change moved to Exited triggers the design cleanup once
// the lock is released.
func (mg *Manager) updateSessionMeta(dir, id string, change func(*sessionMeta) bool) error {
	meta, exitedDesign, err := mg.saveSessionMetaUpdate(dir, id, change)
	if exitedDesign {
		if project, epic, ok := strings.Cut(epicAddress(meta.Ticket), ":"); ok {
			time.Sleep(mg.srv.cfg.SessionExitCleanupDelay)
			mg.designs.cleanupDesign(project, epic)
		}
	}
	return err
}

func (mg *Manager) saveSessionMetaUpdate(dir, id string, change func(*sessionMeta) bool) (meta sessionMeta, exitedDesign bool, err error) {
	mg.watches.metaMu.Lock()
	defer mg.watches.metaMu.Unlock()
	meta, err = readSessionMeta(dir)
	if err != nil {
		return meta, false, fmt.Errorf("read session %s meta: %w", id, err)
	}
	wasExited := meta.Status.State == wire.SessionExited
	if !change(&meta) {
		return meta, false, nil
	}
	if err := writeSessionMeta(dir, meta); err != nil {
		return meta, false, fmt.Errorf("save session %s meta: %w", id, err)
	}
	mg.srv.events.Publish(wire.EventSessionChanged, id)
	return meta, meta.Design && !wasExited && meta.Status.State == wire.SessionExited, nil
}

// updateSessionMetaOrWarn is updateSessionMeta for background callers, which
// have no request to fail.
func (mg *Manager) updateSessionMetaOrWarn(dir, id string, change func(*sessionMeta) bool) {
	if err := mg.updateSessionMeta(dir, id, change); err != nil {
		mg.srv.log.Warn("update session meta", "session", id, "err", err)
	}
}
