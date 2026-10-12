package nativerunner

import (
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
)

// ptyRunnerName is the Runner value a pty agent records in meta.json.
const ptyRunnerName = "pty"

// errPtyNotYet marks a verb the pty runner does not serve yet.
var errPtyNotYet = errors.New("pty runner: not supported yet")

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
	// claudeui.NudgeGrace.
	PromptGrace time.Duration
	// StopGrace is how long Stop waits for the holder to exit before it
	// kills it.
	StopGrace time.Duration
	// Procs stamps pid start times; defaults to ps.
	Procs ProcessTable
	// BackgroundTasks reports whether the claude behind s still runs a
	// background task; defaults to reading its transcript under $HOME.
	BackgroundTasks func(s agentrunner.Session) bool

	mu       sync.Mutex
	sessions map[string]*ptySession
	// starting holds labels whose Start is still launching.
	starting map[string]struct{}
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
	// exited closes once the holder process is gone.
	exited  chan struct{}
	stopped bool
}

func (p *Pty) Name() string { return ptyRunnerName }

func (p *Pty) Start(opts agentrunner.StartOptions) (agentrunner.Session, error) {
	if err := p.reserve(opts.Label); err != nil {
		return agentrunner.Session{}, err
	}
	defer p.release(opts.Label)

	dir := filepath.Join(p.Root, opts.Label)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return agentrunner.Session{}, err
	}
	// Leftovers of an earlier run would stop the holder listening or pass
	// for this claude's exit.
	if err := removeAll(dir, []string{HolderSocketFile, ExitCodeFile}); err != nil {
		return agentrunner.Session{}, err
	}
	sessionID, err := NewUUID()
	if err != nil {
		return agentrunner.Session{}, err
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
		killSessions(ss.claudePID, ss.holderPID)
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
	trusted := false
	for {
		claude, _, err := ss.holder.Status()
		if err != nil {
			return err
		}
		if claude != nil && claude.Status == ClaudeIdle {
			return nil
		}
		if claude == nil && !trusted {
			// The trust dialog shows before claude writes its pid file, so
			// no holder rule can see it; Enter picks "Yes, I trust".
			text, err := p.startupScreen(ss)
			if err != nil {
				return err
			}
			if claudeui.ClassifyStartup(text) == claudeui.StartupTrust {
				if err := ss.holder.SendKeys("\r"); err != nil {
					return err
				}
				trusted = true
			}
		}
		if err := p.launchTick(ss, deadline, "claude never turned idle"); err != nil {
			if claude != nil || !time.Now().After(deadline) {
				return err
			}
			return p.noPidFileErr(ss)
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
	claude := p.Claude
	if claude == "" {
		claude = "claude"
	}
	args := slices.Concat(holder[1:],
		// The holder takes the session id too, so its cmdline passes the
		// liveness check like claude's.
		[]string{"--session-id", sessionID, "--dir", dir, "--", claude},
		ptyLaunchArgs(sessionID, opts.Args))
	cmd := exec.Command(holder[0], args...)
	cmd.Dir = opts.Cwd
	cmd.Env = LaunchEnv(os.Environ(), opts.Env)
	if err := StartDetached(cmd); err != nil {
		return nil, err
	}
	return cmd, nil
}

func ptyLaunchArgs(sessionID string, extra []string) []string {
	return append([]string{
		"--session-id", sessionID,
		"--permission-mode", "auto",
		// The main screen keeps a scrollback to read back.
		"--settings", `{"tui":"default"}`,
		// Agents ask through the ticket's Needs Answer section instead.
		"--disallowedTools", "AskUserQuestion",
	}, extra...)
}

// killSessions SIGKILLs each nonzero pid's whole session.
func killSessions(pids ...int) {
	for _, pid := range pids {
		if pid > 0 {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
	}
}

func (p *Pty) reserve(label string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, live := p.sessions[label]
	if _, starting := p.starting[label]; live || starting {
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
	return ss, nil
}

// died maps a failed holder call to ErrSessionDied: claude's exit code when
// the holder saved one, so claude exited, else the holder failure itself.
func (p *Pty) died(ss *ptySession, err error) error {
	if err == nil {
		return nil
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
	}
	return st, nil
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
		// Longer than the holder's own grace for claude, plus its drain.
		grace = 2 * holderStopGrace
	}
	if ss.holder.Stop() != nil {
		grace = 0
	}
	select {
	case <-ss.exited:
	case <-time.After(grace):
		killSessions(ss.claudePID, ss.holderPID)
		<-ss.exited
	}

	p.mu.Lock()
	delete(p.sessions, s.Label)
	p.mu.Unlock()
	return p.Cleanup(s.Label)
}

// Cleanup removes label's holder.sock, which a crashed holder leaves behind.
func (p *Pty) Cleanup(label string) error {
	return removeAll(filepath.Join(p.Root, label), []string{HolderSocketFile})
}

func (p *Pty) Find(label string) (agentrunner.Session, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ss, ok := p.sessions[label]
	if !ok {
		return agentrunner.Session{}, false, nil
	}
	return ss.session, true, nil
}

func (p *Pty) List(epic string) ([]agentrunner.Session, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []agentrunner.Session
	for _, ss := range p.sessions {
		if ss.epic == epic {
			out = append(out, ss.session)
		}
	}
	slices.SortFunc(out, func(a, b agentrunner.Session) int { return strings.Compare(a.Label, b.Label) })
	return out, nil
}

// Prompt types text plus Enter and returns once Turn moved past its value
// before, nudging and retyping through claudeui.PromptWithNudge.
func (p *Pty) Prompt(s agentrunner.Session, text string) error {
	ss, err := p.live(s)
	if err != nil {
		return err
	}
	_, turn, err := ss.holder.Status()
	if err != nil {
		return p.died(ss, err)
	}
	pr := &ptyPrompter{p: p, ss: ss, turn: turn}
	err = claudeui.PromptWithNudge(pr, text, time.Time{}, time.Now)
	if errors.Is(err, claudeui.ErrStuckSubmission) || errors.Is(err, claudeui.ErrStartTimeout) {
		return fmt.Errorf("%w: %w", agentrunner.ErrNotDelivered, err)
	}
	return err
}

// ptyPrompter is claudeui.Prompter over holder.sock. A turn has started once
// the holder's Turn passes turn.
type ptyPrompter struct {
	p    *Pty
	ss   *ptySession
	turn int
}

func (pr *ptyPrompter) Submit(text string) error {
	if err := pr.ss.holder.SendKeys(text); err != nil {
		return pr.p.died(pr.ss, err)
	}
	return pr.SendEnter()
}

func (pr *ptyPrompter) SendEnter() error { return pr.p.died(pr.ss, pr.ss.holder.SendKeys("\r")) }

// WaitStart caps timeout at PromptGrace, which is how a test keeps the
// nudge rounds short.
func (pr *ptyPrompter) WaitStart(timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(min(timeout, pr.p.promptGrace()))
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

// Interrupt presses Esc, which ends claude's turn and keeps the session, so
// smart-zone can send /compact next.
func (p *Pty) Interrupt(s agentrunner.Session) error {
	ss, err := p.live(s)
	if err != nil {
		return err
	}
	return p.died(ss, ss.holder.SendKeys("\x1b"))
}

// RateLimit never reports a limit: phase 2 has no pty rate-limit rule.
func (p *Pty) RateLimit(s agentrunner.Session) (time.Time, bool, error) {
	_, err := p.live(s)
	return time.Time{}, false, err
}

func (p *Pty) Answer(s agentrunner.Session, _ agentrunner.Answer) error {
	if _, err := p.live(s); err != nil {
		return err
	}
	return fmt.Errorf("%w: answer", errPtyNotYet)
}

func (p *Pty) backgroundTasks(s agentrunner.Session) bool {
	if p.BackgroundTasks != nil {
		return p.BackgroundTasks(s)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	return claudeui.BackgroundTasks(home, time.Now)(s)
}

func (p *Pty) pollInterval() time.Duration {
	if p.PollInterval <= 0 {
		return 100 * time.Millisecond
	}
	return p.PollInterval
}

func (p *Pty) promptGrace() time.Duration {
	if p.PromptGrace <= 0 {
		return claudeui.NudgeGrace
	}
	return p.PromptGrace
}

func (p *Pty) startTimeout() time.Duration {
	if p.StartTimeout <= 0 {
		return 30 * time.Second
	}
	return p.StartTimeout
}
