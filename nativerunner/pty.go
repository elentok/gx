package nativerunner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/harness"
)

// ptyRunnerName is the Runner value a pty agent records in meta.json.
const ptyRunnerName = "pty"

// Pty runs interactive claude in a pty owned by a `gx agent-holder` process,
// started in its own session so it outlives the server. The holder serves
// claude's state and screen over holder.sock.
type Pty struct {
	// Root holds one agent directory per label. Keep it short: holder.sock
	// lives in it and unix socket paths are limited in length.
	Root string
	// Holder is the command that runs `gx agent-holder`; defaults to this
	// executable.
	Holder []string
	// Claude is the claude executable; defaults to "claude" on PATH.
	Claude string
	// Codex is the codex executable; defaults to "codex" on PATH.
	Codex string
	// PollInterval is how often the holder is asked for claude's state.
	PollInterval time.Duration
	// StartTimeout bounds Start's wait for claude to turn idle.
	StartTimeout time.Duration
	// PromptGrace bounds each wait of Prompt for Turn to move; defaults to
	// claudeui.DefaultNudgeCaps.Grace.
	PromptGrace time.Duration
	// StopGrace is how long Stop waits for the holder to exit before it
	// kills it.
	StopGrace time.Duration
	// Procs stamps pid start times; defaults to ps.
	Procs ProcessTable
	// BackgroundTasks reports whether the claude behind s still runs a
	// background task. Nil reads the transcript under the HOME in Env, and
	// means no gating without one.
	BackgroundTasks func(s agentrunner.Session) bool
	// N is the configured runner wait; zero means agentrunner.DefaultN.
	N time.Duration
	// Env (KEY=VALUE) is appended to each holder child's env and is where
	// the typing check reads claude's config dir and home. Empty means no
	// config, never the process env.
	Env []string

	mu       sync.Mutex
	sessions map[string]*ptySession
	// starting holds labels whose Start is still launching.
	starting map[string]struct{}

	envProbeOnce sync.Once
	envProbe     func(agentrunner.Session) bool
}

var (
	_ agentrunner.Runner  = (*Pty)(nil)
	_ agentrunner.Cleaner = (*Pty)(nil)
)

type ptySession struct {
	session   agentrunner.Session
	epic      string
	kind      agentrunner.Kind
	holder    *HolderClient
	holderPID int
	claudePID int
	// The start times meta.json records; blank until launch records them.
	holderStart string
	claudeStart string
	// exited closes once the holder process is gone.
	exited  chan struct{}
	stopped bool
	// died is set once a step left claude in a state gx can't trust; every
	// later call fails with it, so the loop pauses the ticket for repair.
	died error
}

func (p *Pty) Name() string { return ptyRunnerName }

func (p *Pty) Start(opts agentrunner.StartOptions) (agentrunner.Session, error) {
	if err := p.reserve(opts.Label); err != nil {
		return agentrunner.Session{}, err
	}
	defer p.release(opts.Label)

	if s, adopted, err := p.heldLabel(opts); adopted || err != nil {
		return s, err
	}
	dir := filepath.Join(p.Root, opts.Label)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return agentrunner.Session{}, err
	}
	// Leftovers of an earlier run would stop the holder listening or pass
	// for this claude's exit.
	if err := removeAll(dir, []string{HolderSocketFile, ExitCodeFile}); err != nil {
		return agentrunner.Session{}, err
	}
	sessionID := opts.ResumeSessionID
	if sessionID == "" {
		var err error
		if sessionID, err = NewUUID(); err != nil {
			return agentrunner.Session{}, err
		}
	}
	cmd, err := p.spawn(opts, dir, sessionID)
	if err != nil {
		return agentrunner.Session{}, err
	}
	ss := &ptySession{
		session:   agentrunner.Session{Label: opts.Label, ID: opts.Label, SessionID: sessionID},
		epic:      opts.Epic,
		kind:      ptyKind(opts),
		holderPID: cmd.Process.Pid,
		exited:    make(chan struct{}),
	}
	go func() {
		_ = cmd.Wait()
		close(ss.exited)
	}()
	if err := p.launch(ss, dir, opts.Cwd, ptyKind(opts)); err != nil {
		killLaunch(ss.claudePID, ss.holderPID)
		<-ss.exited
		return agentrunner.Session{}, err
	}
	p.mu.Lock()
	if p.sessions == nil {
		p.sessions = map[string]*ptySession{}
	}
	p.sessions[opts.Label] = ss
	p.mu.Unlock()
	return ss.session, nil
}

// launch connects to the holder ss started, records meta.json and waits for
// claude to turn idle.
func (p *Pty) launch(ss *ptySession, dir, cwd string, kind agentrunner.Kind) error {
	h, err := harness.Lookup(kind)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(p.startTimeout())
	for {
		holder, err := DialHolder(dir)
		if err == nil {
			ss.holder, ss.claudePID = holder, holder.ChildPID()
			break
		}
		if err := p.launchTick(ss, deadline, "holder.sock never answered"); err != nil {
			return err
		}
	}
	procs := orPS(p.Procs)
	holderProc, ok, err := procs.Lookup(ss.holderPID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("holder pid %d exited during launch", ss.holderPID)
	}
	meta := Meta{
		Runner:         ptyRunnerName,
		Harness:        kind,
		PID:            ss.claudePID,
		HolderPID:      ss.holderPID,
		HolderPIDStart: holderProc.Start,
		SessionID:      ss.session.SessionID,
		Label:          ss.session.Label,
		Epic:           ss.epic,
		Cwd:            cwd,
	}
	if err := RecordLaunch(procs, dir, &meta); err != nil {
		return err
	}
	ss.holderStart, ss.claudeStart = meta.HolderPIDStart, meta.PIDStart
	// A holder without the watch op can't report ready; it falls back to the
	// pid file's idle.
	watchReady := ss.holder.Supports(OpWatch)
	ready := make(chan struct{})
	if watchReady {
		watch, err := ss.holder.Watch()
		if err != nil {
			return err
		}
		defer watch.Close()
		go func() {
			for {
				m, err := watch.Next()
				if err != nil {
					return
				}
				if m.Type == StreamStatus && m.Ready != "" {
					close(ready)
					return
				}
			}
		}()
	}
	var nextTrustKey time.Time
	for {
		resp, err := ss.holder.statusReply()
		if err != nil {
			return err
		}
		claude := resp.Claude
		if !watchReady && claude != nil && claude.Status == ClaudeIdle {
			return nil
		}
		select {
		case <-ready:
			return nil
		default:
		}
		// A watcher with no pid file says when a dialog is up: its text can
		// stay on screen after it is answered, and a key then would reach the
		// prompt.
		dialogUp := resp.Watched == nil || resp.Watched.State == harness.Blocked && resp.Watched.Blocked == harness.BlockedDialog
		if claude == nil && dialogUp && !time.Now().Before(nextTrustKey) {
			// The trust dialog shows before claude writes its pid file, so
			// no holder rule can see it. Claude drops a key typed before it
			// reads input, so keys repeat while the dialog stays up.
			rows, scrolled, err := ss.holder.Screen(startupScrollback)
			if err != nil {
				return err
			}
			if startup := h.StartupScreen(scrolled, rows); startup.Dialog != harness.NoDialog {
				if err := ss.holder.SendKeys(startup.Key); err != nil {
					return err
				}
				nextTrustKey = time.Now().Add(trustKeyRepeat)
			}
		}
		select {
		case <-ss.exited:
			return p.holderExitedErr(dir)
		case <-time.After(p.pollInterval()):
		}
		if time.Now().After(deadline) {
			if claude == nil {
				return p.noPidFileErr(ss)
			}
			return fmt.Errorf("pty start: claude never turned idle within %v", p.startTimeout())
		}
	}
}

// trustKeyRepeat spaces trust dialog keys so claude redraws the cursor
// before the next key is chosen from the screen.
const trustKeyRepeat = 500 * time.Millisecond

// holderExitedErr reports a holder gone before claude turned idle, with
// claude's exit code when the holder saved one.
func (p *Pty) holderExitedErr(dir string) error {
	code, err := os.ReadFile(filepath.Join(dir, ExitCodeFile))
	if err != nil {
		return errors.New("pty start: holder exited before claude turned idle")
	}
	return fmt.Errorf("pty start: claude exited (code %s) before it turned idle", strings.TrimSpace(string(code)))
}

// Reattach adopts the pty agent a previous server launched under label. The
// holder kept claude's state and Turn, so nothing is replayed. Only core ops
// are used, so a holder from an older gx is adopted too.
//
// A dead pty agent is always VerdictDied, never VerdictFinished: interactive
// claude stays alive after its turn, so it never exits as a normal finish.
//
// The ps lookup and holder dial run unlocked, since they can take seconds and
// would block every other session's calls.
func (p *Pty) Reattach(label string) (agentrunner.Session, Verdict, error) {
	s, outcome, err := p.adopt(label)
	if err != nil {
		return agentrunner.Session{}, 0, err
	}
	if outcome == adoptedLive {
		return s, VerdictLive, nil
	}
	return agentrunner.Session{}, VerdictDied, nil
}

// watchExit closes ss.exited once an adopted holder is gone. It isn't the
// server's child, so there is no Wait to block on.
func (p *Pty) watchExit(ss *ptySession, meta Meta) {
	procs := orPS(p.Procs)
	ticker := time.NewTicker(p.pollInterval())
	defer ticker.Stop()
	for range ticker.C {
		// A failed lookup is retried rather than taken as an exit.
		proc, ok, err := procs.Lookup(meta.HolderPID)
		if err == nil && (!ok || proc.Start != meta.HolderPIDStart) {
			close(ss.exited)
			return
		}
	}
}

// startupScrollback is how many scrolled-off rows a startup screen rule
// reads besides the visible screen.
const startupScrollback = 50

func (p *Pty) startupScreen(ss *ptySession) (string, error) {
	rows, scrolled, err := ss.holder.Screen(startupScrollback)
	if err != nil {
		return "", err
	}
	return claudeui.ScreenText(scrolled, rows), nil
}

// noPidFileErr reports a claude that never wrote its pid status file: either
// it is too old to write one, or it sits on a screen gx can't answer, which
// the error carries for the doctor hint.
func (p *Pty) noPidFileErr(ss *ptySession) error {
	text, err := p.startupScreen(ss)
	if err != nil {
		text = fmt.Sprintf("(unreadable: %v)", err)
	}
	return fmt.Errorf("%w: no pid status file within %v; screen: %s",
		agentrunner.ErrMissingCapability, p.startTimeout(), text)
}

// launchTick sleeps one poll, failing with what once the holder exited or
// deadline passed.
func (p *Pty) launchTick(ss *ptySession, deadline time.Time, what string) error {
	select {
	case <-ss.exited:
		return fmt.Errorf("pty start: holder exited: %s", what)
	case <-time.After(p.pollInterval()):
	}
	if time.Now().After(deadline) {
		return fmt.Errorf("pty start: %s within %v", what, p.startTimeout())
	}
	return nil
}

// spawn starts the holder detached, running claude in its pty.
func (p *Pty) spawn(opts agentrunner.StartOptions, dir, sessionID string) (*exec.Cmd, error) {
	holder := p.Holder
	if len(holder) == 0 {
		exe, err := os.Executable()
		if err != nil {
			return nil, err
		}
		holder = []string{exe, "agent-holder"}
	}
	kind := ptyKind(opts)
	launch, err := p.launchCommand(opts, dir, sessionID)
	if err != nil {
		return nil, err
	}
	args := slices.Concat(holder[1:],
		// The holder takes the session id too, so its cmdline passes the
		// liveness check like the agent's.
		[]string{"--session-id", sessionID, "--harness", string(kind), "--dir", dir, "--", launch.Name},
		launch.Args)
	cmd := exec.Command(holder[0], args...)
	cmd.Dir = opts.Cwd
	cmd.Env = launch.Env
	if err := StartDetached(cmd); err != nil {
		return nil, err
	}
	return cmd, nil
}

// ptyKind is the harness a launch runs; an empty Kind means claude.
func ptyKind(opts agentrunner.StartOptions) agentrunner.Kind {
	if opts.Kind == "" {
		return harness.Claude
	}
	return opts.Kind
}

// launchCommand is the command that runs the agent for opts in its pty.
func (p *Pty) launchCommand(opts agentrunner.StartOptions, dir, sessionID string) (harness.Command, error) {
	kind := ptyKind(opts)
	h, err := harness.Lookup(kind)
	if err != nil {
		return harness.Command{}, err
	}
	args := opts.Args
	if kind == harness.Claude {
		// Only claude takes the statusline settings; another harness's args
		// come from the caller whole.
		if args, err = ptyLaunchArgs(opts.Args, dir); err != nil {
			return harness.Command{}, err
		}
	}
	env := slices.Concat(p.Env, opts.Env)
	if kind == harness.Codex {
		// The SessionStart hook writes Codex's own session id here.
		env = append(env, harness.CodexSessionFileEnv+"="+filepath.Join(dir, harness.CodexSessionFile))
	}
	launchOpts := harness.LaunchOptions{SessionID: sessionID, Args: args, Env: env}
	var launch harness.Command
	if opts.ResumeSessionID != "" {
		// --resume and --session-id fail together, so the agent gets only
		// --resume. The holder still takes --session-id.
		launch, _ = h.Resume(opts.ResumeSessionID, launchOpts)
	} else {
		launch = h.Launch(launchOpts)
	}
	switch kind {
	case harness.Claude:
		if p.Claude != "" {
			launch.Name = p.Claude
		}
	case harness.Codex:
		if p.Codex != "" {
			launch.Name = p.Codex
		}
	}
	return launch, nil
}

func ptyLaunchArgs(extra []string, dir string) ([]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	settings, err := json.Marshal(map[string]any{
		// The main screen keeps a scrollback to read back.
		"tui": "default",
		// The statusline command records the usage limits into the agent dir.
		"statusLine": map[string]any{
			"type":    "command",
			"command": shellQuote(exe) + " claude statusline --agent-dir " + shellQuote(dir),
		},
	})
	if err != nil {
		return nil, err
	}
	return harness.WithGXSystemPrompt(agentrunner.WithLoopDisallowedTools(append([]string{
		"--permission-mode", "auto",
		"--settings", string(settings),
	}, extra...))), nil
}

// shellQuote single-quotes s for the shell that runs the statusline command.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// killLaunch SIGKILLs each nonzero pid's whole session unchecked. Only a
// failed launch uses it: meta.json may not hold the start times yet.
func killLaunch(pids ...int) {
	for _, pid := range pids {
		if pid > 0 {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
	}
}

// killSessions SIGKILLs claude's and the holder's whole sessions, each pid
// re-checked by signalGroup first.
func (p *Pty) killSessions(ss *ptySession) {
	procs := orPS(p.Procs)
	signalGroup(procs, ss.claudePID, ss.claudeStart, ss.session.SessionID, syscall.SIGKILL)
	signalGroup(procs, ss.holderPID, ss.holderStart, ss.session.SessionID, syscall.SIGKILL)
}

// awaitHolderGone polls until ss's holder pid is gone, for at most grace.
func (p *Pty) awaitHolderGone(ss *ptySession, grace time.Duration) error {
	procs := orPS(p.Procs)
	deadline := time.Now().Add(grace)
	for {
		proc, ok, err := procs.Lookup(ss.holderPID)
		if err == nil && (!ok || proc.Start != ss.holderStart) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("stop: pid %d did not exit within %v", ss.holderPID, grace)
		}
		time.Sleep(p.pollInterval())
	}
}

func (p *Pty) reserve(label string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, starting := p.starting[label]; starting {
		return fmt.Errorf("%w: %s", agentrunner.ErrLabelTaken, label)
	}
	if p.starting == nil {
		p.starting = map[string]struct{}{}
	}
	p.starting[label] = struct{}{}
	return nil
}

func (p *Pty) release(label string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.starting, label)
}

func (p *Pty) live(s agentrunner.Session) (*ptySession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ss, ok := p.sessions[s.Label]
	if !ok || ss.session.ID != s.ID {
		return nil, fmt.Errorf("%w: %s", agentrunner.ErrNotFound, s.Label)
	}
	if ss.died != nil {
		return nil, ss.died
	}
	return ss, nil
}

func (p *Pty) markDied(ss *ptySession, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ss.died = err
}

// died maps a failed holder call to ErrNotReady when the holder refused keys
// during a deny-reason, else to ErrSessionDied: claude's exit code when
// the holder reported or saved one, so claude exited, else the holder failure
// itself.
func (p *Pty) died(ss *ptySession, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrHolderDenyBusy) {
		return fmt.Errorf("%w: %v", agentrunner.ErrNotReady, err)
	}
	var exited *ClaudeExitedError
	if errors.As(err, &exited) {
		return agentrunner.SessionDied(exited.Error())
	}
	code, rerr := os.ReadFile(filepath.Join(p.Root, ss.session.Label, ExitCodeFile))
	if rerr == nil {
		return agentrunner.SessionDied(fmt.Sprintf("claude exited (code %s)", strings.TrimSpace(string(code))))
	}
	return agentrunner.SessionDied(fmt.Sprintf("holder failed: %v", err))
}

// Status maps claude's pid status file, as the holder last read it.
func (p *Pty) Status(s agentrunner.Session) (agentrunner.Status, error) {
	ss, err := p.live(s)
	if err != nil {
		return agentrunner.Status{}, err
	}
	resp, err := ss.holder.statusReply()
	if err != nil {
		return agentrunner.Status{}, p.died(ss, err)
	}
	claude := resp.Claude
	st := agentrunner.Status{State: agentrunner.StateWorking, Turn: resp.Turn, SessionID: ss.session.SessionID}
	if w := resp.watched(); w != nil {
		return watchedStatus(st, *w), nil
	}
	if claude == nil {
		return st, nil
	}
	switch claude.Status {
	case ClaudeIdle:
		// A running background task will wake claude again.
		if !p.backgroundTasks(ss.session) {
			st.State = agentrunner.StateIdle
		}
	case ClaudeWaiting:
		st.State, st.BlockedReason = agentrunner.StateBlocked, claude.WaitingFor
		// Only here, on demand: the screen is never read on a timer for it.
		if claude.WaitingFor == permissionPrompt {
			d, err := p.screenDialog(ss)
			if err != nil {
				return agentrunner.Status{}, err
			}
			if d.Approvable {
				st.Pending = &agentrunner.Pending{ID: d.ID, Tool: d.Header, Description: d.Body}
			}
		}
	}
	return st, nil
}

// watchedStatus maps a harness watcher's status, for a harness with no pid
// status file. Working with a background task stays Working: the server reads
// Background off the holder's watch stream. The session id is the one the
// harness reports, not gx's start id, until the harness reports one.
func watchedStatus(st agentrunner.Status, w WatchedStatus) agentrunner.Status {
	if w.ConversationID != "" {
		st.SessionID = w.ConversationID
	}
	switch w.Status.State {
	case harness.Idle:
		st.State = agentrunner.StateIdle
	case harness.Blocked:
		st.State = agentrunner.StateBlocked
		if w.Status.Blocked == harness.BlockedError {
			st.Error = w.Status.Message
		} else {
			st.BlockedReason = w.Status.Message
		}
	}
	return st
}

// screenDialog parses the dialog on the holder's screen right now.
func (p *Pty) screenDialog(ss *ptySession) (claudeui.Dialog, error) {
	rows, _, err := ss.holder.Screen(0)
	if err != nil {
		return claudeui.Dialog{}, p.died(ss, err)
	}
	return claudeui.ParseDialog(rows), nil
}

func (p *Pty) Wait(s agentrunner.Session, states []agentrunner.State, timeout time.Duration) (agentrunner.Status, error) {
	deadline := time.Now().Add(timeout)
	for {
		st, err := p.Status(s)
		if err != nil || slices.Contains(states, st.State) {
			return st, err
		}
		if time.Now().After(deadline) {
			return st, agentrunner.ErrTimeout
		}
		time.Sleep(p.pollInterval())
	}
}

// Stop asks the holder to end claude; a holder that doesn't exit in time is
// killed along with claude.
func (p *Pty) Stop(s agentrunner.Session) error {
	p.mu.Lock()
	ss, ok := p.sessions[s.Label]
	if !ok || ss.session.ID != s.ID || ss.stopped {
		p.mu.Unlock()
		return nil
	}
	ss.stopped = true
	p.mu.Unlock()

	grace := p.StopGrace
	if grace <= 0 {
		// Longer than the holder's own grace for claude, plus its final flush.
		grace = 2 * holderStopGrace
	}
	if ss.holder == nil {
		// A silent holder can't be asked; its exited is already closed, so
		// wait on the pid itself.
		p.killSessions(ss)
		if err := p.awaitHolderGone(ss, grace); err != nil {
			return p.stopFailed(ss, err)
		}
	} else {
		wait := grace
		if ss.holder.Stop() != nil {
			wait = 0
		}
		select {
		case <-ss.exited:
		case <-time.After(wait):
			p.killSessions(ss)
			if err := awaitExit(ss.exited, grace, ss.holderPID); err != nil {
				return p.stopFailed(ss, err)
			}
		}
	}

	p.mu.Lock()
	delete(p.sessions, s.Label)
	p.mu.Unlock()
	return p.Cleanup(s.Label)
}

// stopFailed lets a later Stop retry after err and returns err.
func (p *Pty) stopFailed(ss *ptySession, err error) error {
	p.mu.Lock()
	ss.stopped = false
	p.mu.Unlock()
	return err
}

// Cleanup removes label's holder.sock, which a crashed holder leaves behind.
// It never kills: while the holder may still run, it fails and keeps the
// socket.
func (p *Pty) Cleanup(label string) error {
	dir := filepath.Join(p.Root, label)
	meta, err := readMeta(dir)
	if err != nil && !errors.Is(err, agentrunner.ErrNotFound) {
		return err
	}
	if err == nil {
		live, err := pidAlive(orPS(p.Procs), meta.HolderPID, meta.HolderPIDStart, meta.SessionID)
		if err != nil {
			return fmt.Errorf("cleanup %s: holder pid %d: %w", label, meta.HolderPID, err)
		}
		if live {
			return fmt.Errorf("cleanup %s: holder pid %d still runs", label, meta.HolderPID)
		}
	}
	return removeAll(dir, layouts[ptyRunnerName].transient)
}

var _ agentrunner.Nudger = (*Pty)(nil)

// Prompt types text plus Enter and returns once Turn moved past its value
// before, nudging and retyping through claudeui.PromptWithNudge.
func (p *Pty) Prompt(s agentrunner.Session, text string) error {
	return p.PromptNudged(s, text, nil)
}

func (p *Pty) PromptNudged(s agentrunner.Session, text string, onNudge func(string)) error {
	ss, err := p.live(s)
	if err != nil {
		return err
	}
	turn, err := p.awaitNotBusy(ss)
	if err != nil {
		return err
	}
	pr := &ptyPrompter{p: p, ss: ss, turn: turn}
	// Without the watch op a person's keys go unseen, as on any older holder.
	if person, stop, err := p.PersonInput(s); err == nil {
		defer stop()
		pr.person = person
	}
	if err := pr.awaitEmptyInput(p.promptGrace()); err != nil {
		return notDelivered(err)
	}
	if isSlashCommand(text) {
		err = p.promptSlash(pr, text)
	} else {
		// One bare-Enter nudge and one retype, then ErrNotDelivered: herdr's caps
		// would hold a stuck prompt for minutes before the loop hears of it.
		caps := claudeui.NudgeCaps{Grace: p.promptGrace(), Nudges: 2, Retypes: 1}
		err = claudeui.PromptWithNudge(pr, text, caps, time.Time{}, time.Now, claudeui.ReportDecisions(onNudge))
	}
	if errors.Is(err, errPersonInput) {
		return nil
	}
	return notDelivered(err)
}

func notDelivered(err error) error {
	if errors.Is(err, claudeui.ErrStuckSubmission) || errors.Is(err, claudeui.ErrStartTimeout) {
		return fmt.Errorf("%w: %w", agentrunner.ErrNotDelivered, err)
	}
	return err
}

// isSlashCommand reports whether text is a "/" command that takes the
// single-send rule. /compact keeps the normal path: it starts a turn.
func isSlashCommand(text string) bool {
	return strings.HasPrefix(text, "/") && text != "/compact"
}

// promptSlash sends a "/" command once, never nudging or retyping: most
// commands start no turn, so a retype would run them twice. Delivered means
// Turn moved (a working report, a new sessionId, a dialog) or claude is still
// idle after N. A dialog command leaves the session blocked; gx never closes
// it.
func (p *Pty) promptSlash(pr *ptyPrompter, text string) error {
	if err := pr.Submit(text); err != nil {
		return err
	}
	started, err := pr.WaitStart(p.WaitN())
	if err != nil || started {
		return err
	}
	claude, _, program, err := pr.ss.holder.statusWithProgram()
	if err != nil {
		return p.died(pr.ss, err)
	}
	if program == "working" || (claude != nil && claude.Status == ClaudeBusy) {
		return fmt.Errorf("%w: %q started no turn and claude is not idle", agentrunner.ErrNotDelivered, text)
	}
	return nil
}

// awaitNotBusy waits up to N for claude to stop being busy and returns the
// Turn to send against. A prompt typed into a busy claude is only queued, so
// its next idle edge would pass for delivery. A background task alone is not
// busy.
func (p *Pty) awaitNotBusy(ss *ptySession) (int, error) {
	deadline := time.Now().Add(p.WaitN())
	for {
		resp, err := ss.holder.statusReply()
		if err != nil {
			return 0, p.died(ss, err)
		}
		claude, turn, program := resp.Claude, resp.Turn, resp.Program
		// A watched harness (Codex) reports Background itself: working with a
		// background terminal takes a prompt, a running turn does not.
		if w := resp.watched(); w != nil {
			switch {
			// After an error the input is ready: a prompt starts the next turn.
			case w.Status.State == harness.Blocked && w.Status.Blocked != harness.BlockedError:
				return 0, fmt.Errorf("%w: blocked on %s", agentrunner.ErrNotReady, w.Status.Message)
			case w.Status.State != harness.Working || w.Status.Background:
				return turn, nil
			}
			claude, program = nil, "working"
		}
		// Enter on a dialog would pick its first option, allowing a permission
		// prompt nobody answered.
		if claude != nil && claude.Status == ClaudeWaiting {
			return 0, fmt.Errorf("%w: blocked on %s", agentrunner.ErrNotReady, claude.WaitingFor)
		}
		// The OSC report leads the pid file by up to a poll.
		if program != "working" && (claude == nil || claude.Status != ClaudeBusy) {
			return turn, nil
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("%w: busy for %s", agentrunner.ErrNotReady, p.WaitN())
		}
		select {
		case <-ss.exited:
			return 0, p.died(ss, errors.New("holder exited"))
		case <-time.After(p.pollInterval()):
		}
	}
}

// Interrupt picks its key by state: Ctrl+C when busy, a lone Esc when blocked
// (it denies the dialog), nothing otherwise. After a key it waits up to N for
// idle, then presses Ctrl+C once: claude restores the cancelled prompt into the
// input line, and it would prefix the next prompt. Ctrl+U clears one line per
// press, so it can't empty a multi-line prompt; Ctrl+C on an idle prompt clears
// it all. It is sent once and only after idle, since a second quick Ctrl+C
// quits claude. The session is kept, so smart-zone can send /compact next with
// no further wait.
func (p *Pty) Interrupt(s agentrunner.Session) error {
	ss, err := p.live(s)
	if err != nil {
		return err
	}
	resp, err := ss.holder.statusReply()
	if err != nil {
		return p.died(ss, err)
	}
	claude, program := resp.Claude, resp.Program
	w := resp.watched()
	if w != nil {
		program = watchedProgram(w.Status)
	}
	var key string
	switch {
	case program == "blocked" || (program == "" && claude != nil && claude.Status == ClaudeWaiting):
		key = "\x1b"
	case program == "working" || (program == "" && claude != nil && claude.Status == ClaudeBusy):
		key = "\x03"
	default:
		return nil
	}
	if err := ss.holder.SendKeys(key); err != nil {
		return p.died(ss, err)
	}
	if err := p.awaitIdle(ss); err != nil {
		return err
	}
	if w != nil {
		// A second Ctrl+C would quit Codex.
		return nil
	}
	return p.died(ss, ss.holder.SendKeys("\x03"))
}

// watchedProgram maps a watched harness's status to the OSC state names the
// pid-file paths use: a background terminal alone is idle.
func watchedProgram(st harness.Status) string {
	switch {
	case st.State == harness.Blocked:
		return "blocked"
	case st.State == harness.Working && !st.Background:
		return "working"
	}
	return "idle"
}

// awaitIdle waits up to N for claude to be neither busy nor blocked.
func (p *Pty) awaitIdle(ss *ptySession) error {
	deadline := time.Now().Add(p.WaitN())
	for {
		resp, err := ss.holder.statusReply()
		if err != nil {
			return p.died(ss, err)
		}
		claude, program := resp.Claude, resp.Program
		if w := resp.watched(); w != nil {
			program = watchedProgram(w.Status)
		}
		busy := program == "working" || program == "blocked" ||
			(program == "" && claude != nil && (claude.Status == ClaudeBusy || claude.Status == ClaudeWaiting))
		if !busy {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: not idle after %s", agentrunner.ErrNotReady, p.WaitN())
		}
		select {
		case <-ss.exited:
			return p.died(ss, errors.New("holder exited"))
		case <-time.After(p.pollInterval()):
		}
	}
}

// permissionPrompt is the waitingFor of both tool permission and plan
// approval. Their first option is "Yes", so Enter allows.
const permissionPrompt = "permission prompt"

// Answer presses Enter to allow a permission prompt or Esc to deny any
// dialog, and returns once Turn moved. Allow on any other dialog is
// ErrInvalidDecision: its options aren't known, but Esc always cancels it.
// Deny with a reason also sends the reason as claude's next prompt. With a
// RequestID, the screen is re-read first and anything but that approvable
// dialog is ErrRequestMismatch, before any key is sent.
func (p *Pty) Answer(s agentrunner.Session, a agentrunner.Answer) error {
	ss, err := p.live(s)
	if err != nil {
		return err
	}
	claude, turn, err := ss.holder.Status()
	if err != nil {
		return p.died(ss, err)
	}
	if claude == nil || claude.Status != ClaudeWaiting {
		return fmt.Errorf("%w: answer on a session that is not blocked", agentrunner.ErrNotReady)
	}
	if a.RequestID != "" {
		d, err := p.screenDialog(ss)
		if err != nil {
			return err
		}
		if !d.Approvable || d.ID != a.RequestID {
			return fmt.Errorf("%w: %s", agentrunner.ErrRequestMismatch, a.RequestID)
		}
	}
	var key string
	switch a.Decision {
	case agentrunner.DecisionAllow:
		if claude.WaitingFor != permissionPrompt {
			return fmt.Errorf("%w: %q is not a permission prompt; deny cancels it",
				agentrunner.ErrInvalidDecision, claude.WaitingFor)
		}
		key = "\r"
	case agentrunner.DecisionDeny:
		if a.Text != "" {
			return p.denyWithReason(ss, a.Text)
		}
		key = "\x1b"
	default:
		return fmt.Errorf("%w: %q", agentrunner.ErrInvalidDecision, a.Decision)
	}
	if err := p.died(ss, ss.holder.SendKeys(key)); err != nil {
		return err
	}
	pr := &ptyPrompter{p: p, ss: ss, turn: turn}
	moved, err := pr.WaitStart(p.promptGrace())
	if err != nil {
		return err
	}
	if !moved {
		return fmt.Errorf("%w: %s left the dialog open", agentrunner.ErrNotDelivered, a.Decision)
	}
	return nil
}

// denyWithReason prefers the holder's deny-reason op, which keeps Status
// blocked until claude takes the reason; between a bare Esc and the reason,
// a waiter would see idle and end the turn. A failed step leaves claude
// somewhere between the dialog and the reason, so the session is marked died.
func (p *Pty) denyWithReason(ss *ptySession, reason string) error {
	var err error
	if ss.holder.Supports(OpDenyReason) {
		err = p.died(ss, ss.holder.DenyWithReason(reason))
	} else {
		err = p.escThenPrompt(ss, reason)
	}
	if errors.Is(err, agentrunner.ErrSessionDied) {
		p.markDied(ss, err)
	}
	return err
}

// escThenPrompt is deny-reason over core ops, for a holder that predates the
// op: Esc, then the reason as a normal prompt once claude is idle.
func (p *Pty) escThenPrompt(ss *ptySession, reason string) error {
	if err := p.died(ss, ss.holder.SendKeys("\x1b")); err != nil {
		return err
	}
	deadline := time.Now().Add(p.promptGrace())
	for {
		claude, _, err := ss.holder.Status()
		if err != nil {
			return p.died(ss, err)
		}
		if claude != nil && claude.Status == ClaudeIdle {
			return p.Prompt(ss.session, reason)
		}
		if time.Now().After(deadline) {
			return agentrunner.SessionDied("claude did not go idle after Esc")
		}
		time.Sleep(p.pollInterval())
	}
}

func (p *Pty) backgroundTasks(s agentrunner.Session) bool {
	if p.BackgroundTasks != nil {
		return p.BackgroundTasks(s)
	}
	p.envProbeOnce.Do(func() {
		if home := envValue(p.Env, "HOME"); home != "" {
			p.envProbe = claudeui.BackgroundTasks(home, time.Now)
		}
	})
	return p.envProbe != nil && p.envProbe(s)
}

// envValue is the last value of key in a KEY=VALUE list, as exec resolves
// duplicates.
func envValue(env []string, key string) string {
	var v string
	for _, kv := range env {
		if k, val, ok := strings.Cut(kv, "="); ok && k == key {
			v = val
		}
	}
	return v
}

func (p *Pty) pollInterval() time.Duration {
	if p.PollInterval <= 0 {
		return 100 * time.Millisecond
	}
	return p.PollInterval
}

// WaitN is N, the configured runner wait.
func (p *Pty) WaitN() time.Duration {
	if p.N <= 0 {
		return agentrunner.DefaultN
	}
	return p.N
}

func (p *Pty) promptGrace() time.Duration {
	if p.PromptGrace <= 0 {
		return claudeui.DefaultNudgeCaps.Grace
	}
	return p.PromptGrace
}

func (p *Pty) startTimeout() time.Duration {
	if p.StartTimeout <= 0 {
		return 30 * time.Second
	}
	return p.StartTimeout
}
