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
	// background task; defaults to reading its transcript under $HOME.
	BackgroundTasks func(s agentrunner.Session) bool
	// N is the configured runner wait; zero means agentrunner.DefaultN.
	N time.Duration

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
	// The start times meta.json records; blank until launch records them.
	holderStart string
	claudeStart string
	// exited closes once the holder process is gone.
	exited  chan struct{}
	stopped bool
	// died is set once a step left claude in a state gx can't trust; every
	// later call fails with it, so the loop parks the ticket needs-repair.
	died error
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
	var nextTrustKey time.Time
	for {
		claude, _, err := ss.holder.Status()
		if err != nil {
			return err
		}
		if claude != nil && claude.Status == ClaudeIdle {
			return nil
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
	if s, ok, _ := p.Find(label); ok {
		return s, VerdictLive, nil
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
		return agentrunner.Session{}, VerdictDied, nil
	}
	// A holder whose socket stays silent can't be driven, so it counts as dead.
	holder, err := DialHolder(dir)
	if err != nil {
		return agentrunner.Session{}, VerdictDied, nil
	}
	ss := &ptySession{
		session:     agentrunner.Session{Label: label, ID: label, SessionID: meta.SessionID},
		epic:        meta.Epic,
		holder:      holder,
		holderPID:   meta.HolderPID,
		claudePID:   meta.PID,
		holderStart: meta.HolderPIDStart,
		claudeStart: meta.PIDStart,
		exited:      make(chan struct{}),
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	// A concurrent Reattach of label may have adopted it meanwhile.
	if other, ok := p.sessions[label]; ok {
		return other.session, VerdictLive, nil
	}
	go p.watchExit(ss, meta)
	if p.sessions == nil {
		p.sessions = map[string]*ptySession{}
	}
	p.sessions[label] = ss
	return ss.session, VerdictLive, nil
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
	launch := ptyHarness.Launch(harness.LaunchOptions{SessionID: sessionID, Args: ptyLaunchArgs(opts.Args), Env: opts.Env})
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

func ptyLaunchArgs(extra []string) []string {
	return append([]string{
		"--permission-mode", "auto",
		// The main screen keeps a scrollback to read back.
		"--settings", `{"tui":"default"}`,
		// Agents ask through the ticket's Needs Answer section instead.
		"--disallowedTools", "AskUserQuestion",
	}, extra...)
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

// died maps a failed holder call to ErrSessionDied: claude's exit code when
// the holder reported or saved one, so claude exited, else the holder failure
// itself.
func (p *Pty) died(ss *ptySession, err error) error {
	if err == nil {
		return nil
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
	// One bare-Enter nudge and one retype, then ErrNotDelivered: herdr's caps
	// would hold a stuck prompt for minutes before the loop hears of it.
	caps := claudeui.NudgeCaps{Grace: p.promptGrace(), Nudges: 2, Retypes: 1}
	err = claudeui.PromptWithNudge(pr, text, caps, time.Time{}, time.Now, claudeui.ReportDecisions(onNudge))
	if errors.Is(err, claudeui.ErrStuckSubmission) || errors.Is(err, claudeui.ErrStartTimeout) {
		return fmt.Errorf("%w: %w", agentrunner.ErrNotDelivered, err)
	}
	return err
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
	if err := pr.ss.holder.SendKeys(text); err != nil {
		return pr.p.died(pr.ss, err)
	}
	return pr.SendEnter()
}

// Clear presses Ctrl+U, as herdr's does.
func (pr *ptyPrompter) Clear() error { return pr.p.died(pr.ss, pr.ss.holder.SendKeys("\x15")) }

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

// permissionPrompt is the waitingFor of both tool permission and plan
// approval. Their first option is "Yes", so Enter allows.
const permissionPrompt = "permission prompt"

// Answer presses Enter to allow a permission prompt or Esc to deny any
// dialog, and returns once Turn moved. Allow on any other dialog is
// ErrInvalidDecision: its options aren't known, but Esc always cancels it.
// Deny with a reason also sends the reason as claude's next prompt.
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
