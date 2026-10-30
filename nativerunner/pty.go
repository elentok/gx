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
		holderPID: cmd.Process.Pid,
		exited:    make(chan struct{}),
	}
	go func() {
		_ = cmd.Wait()
		close(ss.exited)
	}()
	if err := p.launch(ss, dir, opts.Cwd); err != nil {
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
func (p *Pty) launch(ss *ptySession, dir, cwd string) error {
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
		claude, _, err := ss.holder.Status()
		if err != nil {
			return err
		}
		if !watchReady && claude != nil && claude.Status == ClaudeIdle {
			return nil
		}
		select {
		case <-ready:
			return nil
		default:
		}
		if claude == nil && !time.Now().Before(nextTrustKey) {
			// The trust dialog shows before claude writes its pid file, so
			// no holder rule can see it. Claude drops a key typed before it
			// reads input, so keys repeat while the dialog stays up.
			rows, scrolled, err := ss.holder.Screen(startupScrollback)
			if err != nil {
				return err
			}
			if startup := ptyHarness.StartupScreen(scrolled, rows); startup.Dialog == harness.TrustDialog {
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

type adoption int

const (
	adoptGone   adoption = iota // holder pid is not alive
	adoptSilent                 // holder pid is alive but its socket doesn't answer
	adoptedLive                 // session is tracked and drivable
)

// tracked returns label's drivable in-memory session. A silent one that Find
// registered as dead doesn't count, so Reattach still re-checks it.
func (p *Pty) tracked(label string) (agentrunner.Session, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ss, ok := p.sessions[label]
	if !ok || ss.died != nil {
		return agentrunner.Session{}, false
	}
	return ss.session, true
}

// adopt tracks the holder under label's dir. It is the one adopt path for
// Reattach, Find and List.
func (p *Pty) adopt(label string) (agentrunner.Session, adoption, error) {
	if s, ok := p.tracked(label); ok {
		return s, adoptedLive, nil
	}
	dir := filepath.Join(p.Root, label)
	meta, err := readMeta(dir)
	if err != nil {
		return agentrunner.Session{}, 0, err
	}
	live, err := ptyAlive(orPS(p.Procs), meta)
	if err != nil {
		return agentrunner.Session{}, 0, err
	}
	if !live {
		return agentrunner.Session{}, adoptGone, nil
	}
	// A holder whose socket stays silent can't be driven.
	holder, err := DialHolder(dir)
	if err != nil {
		return p.trackSilent(label, meta), adoptSilent, nil
	}
	ss := p.newSession(label, meta)
	ss.holder = holder
	p.mu.Lock()
	defer p.mu.Unlock()
	// A concurrent adopt of label may have won meanwhile.
	if other, ok := p.sessions[label]; ok && other.died == nil {
		return other.session, adoptedLive, nil
	}
	go p.watchExit(ss, meta)
	if p.sessions == nil {
		p.sessions = map[string]*ptySession{}
	}
	p.sessions[label] = ss
	return ss.session, adoptedLive, nil
}

func (p *Pty) newSession(label string, meta Meta) *ptySession {
	return &ptySession{
		session:     agentrunner.Session{Label: label, ID: label, SessionID: meta.SessionID},
		epic:        meta.Epic,
		holderPID:   meta.HolderPID,
		claudePID:   meta.PID,
		holderStart: meta.HolderPIDStart,
		claudeStart: meta.PIDStart,
		exited:      make(chan struct{}),
	}
}

// trackSilent registers a live-pid holder that doesn't answer as a dead
// session, so every call on it fails with ErrSessionDied.
func (p *Pty) trackSilent(label string, meta Meta) agentrunner.Session {
	ss := p.newSession(label, meta)
	ss.died = agentrunner.SessionDied("holder socket is silent")
	close(ss.exited)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sessions == nil {
		p.sessions = map[string]*ptySession{}
	}
	p.sessions[label] = ss
	return ss.session
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
	claudeArgs, err := ptyLaunchArgs(opts.Args, dir)
	if err != nil {
		return nil, err
	}
	launchOpts := harness.LaunchOptions{SessionID: sessionID, Args: claudeArgs, Env: slices.Concat(p.Env, opts.Env)}
	var launch harness.Command
	if opts.ResumeSessionID != "" {
		// --resume and --session-id fail together, so claude gets only --resume.
		// The holder still takes --session-id below.
		launch, _ = ptyHarness.Resume(opts.ResumeSessionID, launchOpts)
	} else {
		launch = ptyHarness.Launch(launchOpts)
	}
	if p.Claude != "" {
		launch.Name = p.Claude
	}
	args := slices.Concat(holder[1:],
		// The holder takes the session id too, so its cmdline passes the
		// liveness check like claude's.
		[]string{"--session-id", sessionID, "--harness", string(harness.Claude), "--dir", dir, "--", launch.Name},
		launch.Args)
	cmd := exec.Command(holder[0], args...)
	cmd.Dir = opts.Cwd
	cmd.Env = launch.Env
	if err := StartDetached(cmd); err != nil {
		return nil, err
	}
	return cmd, nil
}

// ptyHarness is the loop's harness: always claude.
var ptyHarness = func() harness.Harness {
	h, err := harness.Lookup(harness.Claude)
	if err != nil {
		panic(err)
	}
	return h
}()

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

// heldLabel decides what a live holder under opts.Label means for Start: the
// holder's session when it is adopted, a LabelTakenError when it blocks the
// launch, and neither when the dir has no live holder. It never touches the
// holder's files.
func (p *Pty) heldLabel(opts agentrunner.StartOptions) (agentrunner.Session, bool, error) {
	s, outcome, err := p.adopt(opts.Label)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return agentrunner.Session{}, false, nil
	}
	if err != nil {
		return agentrunner.Session{}, false, err
	}
	if outcome == adoptGone {
		return agentrunner.Session{}, false, nil
	}
	meta, err := readMeta(filepath.Join(p.Root, opts.Label))
	if err != nil {
		return agentrunner.Session{}, false, err
	}
	if outcome == adoptedLive && !opts.NoAdopt && filepath.Clean(meta.Cwd) == filepath.Clean(opts.Cwd) {
		return s, true, nil
	}
	return agentrunner.Session{}, false, &agentrunner.LabelTakenError{
		HolderCwd: meta.Cwd,
		Err:       fmt.Errorf("%s: holder pid %d runs in %s", opts.Label, meta.HolderPID, meta.Cwd),
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
	claude, turn, err := ss.holder.Status()
	if err != nil {
		return agentrunner.Status{}, p.died(ss, err)
	}
	st := agentrunner.Status{State: agentrunner.StateWorking, Turn: turn, SessionID: ss.session.SessionID}
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
			p.mu.Lock()
			ss.stopped = false
			p.mu.Unlock()
			return err
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
				p.mu.Lock()
				ss.stopped = false
				p.mu.Unlock()
				return err
			}
		}
	}

	p.mu.Lock()
	delete(p.sessions, s.Label)
	p.mu.Unlock()
	return p.Cleanup(s.Label)
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

// Find returns label's agent, adopting a live holder an earlier server left
// behind. A holder with a live pid and a silent socket is returned dead.
func (p *Pty) Find(label string) (agentrunner.Session, bool, error) {
	p.mu.Lock()
	ss, ok := p.sessions[label]
	p.mu.Unlock()
	if ok {
		return ss.session, true, nil
	}
	s, outcome, err := p.adopt(label)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return agentrunner.Session{}, false, nil
	}
	if err != nil {
		return agentrunner.Session{}, false, err
	}
	if outcome == adoptGone {
		return agentrunner.Session{}, false, nil
	}
	return s, true, nil
}

// List returns epic's agents, adopting live holders on disk that this value
// doesn't track yet.
func (p *Pty) List(epic string) ([]agentrunner.Session, error) {
	labels := map[string]struct{}{}
	p.mu.Lock()
	for label, ss := range p.sessions {
		if ss.epic == epic {
			labels[label] = struct{}{}
		}
	}
	p.mu.Unlock()
	entries, err := os.ReadDir(p.Root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		meta, err := readMeta(filepath.Join(p.Root, e.Name()))
		if err == nil && meta.Epic == epic {
			labels[e.Name()] = struct{}{}
		}
	}
	var out []agentrunner.Session
	for label := range labels {
		s, ok, err := p.Find(label)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(a, b agentrunner.Session) int { return strings.Compare(a.Label, b.Label) })
	return out, nil
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
	if isSlashCommand(text) {
		return p.promptSlash(pr, text)
	}
	// One bare-Enter nudge and one retype, then ErrNotDelivered: herdr's caps
	// would hold a stuck prompt for minutes before the loop hears of it.
	caps := claudeui.NudgeCaps{Grace: p.promptGrace(), Nudges: 2, Retypes: 1}
	err = claudeui.PromptWithNudge(pr, text, caps, time.Time{}, time.Now, claudeui.ReportDecisions(onNudge))
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
		claude, turn, program, err := ss.holder.statusWithProgram()
		if err != nil {
			return 0, p.died(ss, err)
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

// ptyPrompter is claudeui.Prompter over holder.sock. A turn has started once
// the holder's Turn passes turn.
type ptyPrompter struct {
	p    *Pty
	ss   *ptySession
	turn int
}

func (pr *ptyPrompter) Submit(text string) error {
	if err := pr.ss.holder.TypeText(text); err != nil {
		return pr.p.died(pr.ss, err)
	}
	return pr.SendEnter()
}

// Clear presses the harness clear key once.
func (pr *ptyPrompter) Clear() error {
	return pr.p.died(pr.ss, pr.ss.holder.SendKeys(pr.p.typing().ClearKey()))
}

// InputEmpty runs the harness empty check when the holder serves styled rows.
func (pr *ptyPrompter) InputEmpty() (bool, bool, error) {
	if !pr.ss.holder.Supports(OpScreenStyled) {
		return false, false, nil
	}
	rows, err := pr.ss.holder.StyledScreen()
	if err != nil {
		return false, false, pr.p.died(pr.ss, err)
	}
	switch pr.p.typing().InputIsEmpty(rows) {
	case harness.InputEmpty:
		return true, true, nil
	case harness.InputNotEmpty:
		return false, true, nil
	}
	return false, false, nil
}

// ReadyToType runs claude's ready-to-type step over the holder.
func (pr *ptyPrompter) ReadyToType() bool {
	return pr.p.typing().ReadyToType(HolderTypingIO(pr.ss.holder))
}

// HolderTypingIO reads styled rows when the holder serves them, plain rows
// otherwise; the ready-to-type step needs only the text.
func HolderTypingIO(h *HolderClient) harness.TypingIO {
	return harness.TypingIO{
		Rows: func() ([]string, error) {
			if h.Supports(OpScreenStyled) {
				return h.StyledScreen()
			}
			rows, _, err := h.Screen(0)
			return rows, err
		},
		SendKeys: h.SendKeys,
	}
}

// claudeTyping is the holder child's typing rules; it reads the env the
// parent gave that child.
var claudeTyping = harness.ClaudeTyping()

// typing is claude's typing rules reading its config from p.Env. A nil Env
// becomes an empty one, which harness would read as the process env.
func (p *Pty) typing() harness.Typing {
	env := p.Env
	if env == nil {
		env = []string{}
	}
	return harness.ClaudeTypingIn(env)
}

func (pr *ptyPrompter) SendEnter() error { return pr.p.died(pr.ss, pr.ss.holder.SendKeys("\r")) }

func (pr *ptyPrompter) WaitStart(timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		_, turn, err := pr.ss.holder.Status()
		if err != nil {
			return false, pr.p.died(pr.ss, err)
		}
		if turn > pr.turn {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		select {
		case <-pr.ss.exited:
			return false, pr.p.died(pr.ss, errors.New("holder exited"))
		case <-time.After(pr.p.pollInterval()):
		}
	}
}

func (pr *ptyPrompter) Screen() (string, error) {
	rows, _, err := pr.ss.holder.Screen(0)
	return strings.Join(rows, "\n"), pr.p.died(pr.ss, err)
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
	claude, _, program, err := ss.holder.statusWithProgram()
	if err != nil {
		return p.died(ss, err)
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
	return p.died(ss, ss.holder.SendKeys("\x03"))
}

// awaitIdle waits up to N for claude to be neither busy nor blocked.
func (p *Pty) awaitIdle(ss *ptySession) error {
	deadline := time.Now().Add(p.WaitN())
	for {
		claude, _, program, err := ss.holder.statusWithProgram()
		if err != nil {
			return p.died(ss, err)
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

// RateLimit reads the limit from the transcript fact the holder reports, not
// the screen. A message without a parseable reset time gives a zero time.
func (p *Pty) RateLimit(s agentrunner.Session) (time.Time, bool, error) {
	ss, err := p.live(s)
	if err != nil {
		return time.Time{}, false, err
	}
	message, limited, err := ss.holder.RateLimitMessage()
	if err != nil {
		return time.Time{}, false, p.died(ss, err)
	}
	if !limited {
		return time.Time{}, false, nil
	}
	now := time.Now()
	if d, ok := claudeui.SecondsUntilReset(claudeui.ResetToken(message), now); ok {
		return now.Add(d), true, nil
	}
	return p.usageRecordReset(ss.session.Label), true, nil
}

// usageLimitPercent is the fixed usage at which a record window counts as the
// limit that fired.
const usageLimitPercent = 98

// usageRecordReset is the latest resets_at among label's record windows at
// usageLimitPercent or more, or zero when there is no such window. A missing
// or unreadable record gives zero: the record only refines the message.
func (p *Pty) usageRecordReset(label string) time.Time {
	data, err := os.ReadFile(filepath.Join(p.Root, label, UsageFile))
	if err != nil {
		return time.Time{}
	}
	var rec UsageRecord
	if json.Unmarshal(data, &rec) != nil {
		return time.Time{}
	}
	var latest int64
	for _, w := range []*UsageWindow{rec.FiveHour, rec.SevenDay} {
		if w != nil && w.UsedPercentage >= usageLimitPercent {
			latest = max(latest, w.ResetsAt)
		}
	}
	if latest == 0 {
		return time.Time{}
	}
	return time.Unix(latest, 0)
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
