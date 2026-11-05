package nativerunner

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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
	// LaunchCaps bound a launch prompt's transcript proof; a zero field means
	// the default.
	LaunchCaps claudeui.LaunchProofCaps
	// ReadyWait caps a launch prompt's wait for the ready signal; zero means
	// DefaultReadyWait.
	ReadyWait time.Duration

	// log receives the vim probe's decisions (see SetLog); nil logs nothing.
	log *slog.Logger

	reg ptyRegistry

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
	// sentPresses is the probe's `i` count at the last send: an `i` pressed
	// after it may still sit in the input.
	sentPresses atomic.Int64
	// died is set once a step left claude in a state gx can't trust; every
	// later call fails with it, so the loop pauses the ticket for repair.
	died error
}

func (p *Pty) Name() string { return ptyRunnerName }

// SetLog names the logger for the vim probe's decisions. The server owns the
// logger and builds its runners before it exists, so it is set after, before
// the runner's first use.
func (p *Pty) SetLog(l *slog.Logger) { p.log = l }

func (p *Pty) Start(opts agentrunner.StartOptions) (agentrunner.Session, error) {
	if err := p.reg.reserve(opts.Label); err != nil {
		return agentrunner.Session{}, err
	}
	defer p.reg.release(opts.Label)

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
	l := p.launcher()
	cmd, err := l.spawn(opts, dir, sessionID)
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
	if err := l.launch(ss, dir, opts.Cwd, ptyKind(opts)); err != nil {
		killLaunch(ss.claudePID, ss.holderPID)
		<-ss.exited
		return agentrunner.Session{}, err
	}
	p.reg.put(ss)
	return ss.session, nil
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

// ptyKind is the harness a launch runs; an empty Kind means claude.
func ptyKind(opts agentrunner.StartOptions) agentrunner.Kind {
	if opts.Kind == "" {
		return harness.Claude
	}
	return opts.Kind
}

func ptyLaunchArgs(extra []string, dir string) ([]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	settings, err := json.Marshal(map[string]any{
		// The main screen keeps a scrollback to read back.
		"tui": "default",
		// The claude.ai synced skills (docx, pptx, ...) are never used and cost
		// about 3k tokens in every turn's prefix. claude reads only the last
		// --settings flag, so a caller's own --settings replaces this whole map.
		"syncClaudeAiSkills": false,
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
	ss, err := p.reg.live(s)
	if err != nil {
		return agentrunner.Status{}, err
	}
	resp, err := ss.holder.StatusReply()
	if err != nil {
		return agentrunner.Status{}, p.died(ss, err)
	}
	claude := resp.Claude
	st := agentrunner.Status{State: agentrunner.StateWorking, Turn: resp.Turn, SessionID: ss.session.SessionID}
	if w := resp.WatchedView(); w != nil {
		st = watchedStatus(st, *w)
		if st.State == agentrunner.StateBlocked && w.Status.Blocked == harness.BlockedPermission {
			if st.Pending, err = p.codexPending(ss); err != nil {
				return agentrunner.Status{}, err
			}
		}
		return st, nil
	}
	if claude == nil {
		return st, nil
	}
	switch claude.Status {
	case ClaudeIdle:
		// A running background task will wake claude again.
		if !p.turns().backgroundTasks(ss.session) {
			st.State = agentrunner.StateIdle
		}
	case ClaudeWaiting:
		st.State, st.BlockedReason = agentrunner.StateBlocked, claude.WaitingFor
		// Only here, on demand: the screen is never read on a timer for it.
		if claude.WaitingFor == PermissionPrompt {
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
	ss := p.reg.claimStop(s)
	if ss == nil {
		return nil
	}

	grace := p.StopGrace
	if grace <= 0 {
		// Longer than the holder's own grace for claude, plus its final flush.
		grace = 2 * HolderStopGrace
	}
	if ss.holder == nil {
		// A silent holder can't be asked; its exited is already closed, so
		// wait on the pid itself.
		p.killSessions(ss)
		if err := p.awaitHolderGone(ss, grace); err != nil {
			return p.reg.stopFailed(ss, err)
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
				return p.reg.stopFailed(ss, err)
			}
		}
	}

	p.reg.remove(s.Label)
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

var _ agentrunner.Nudger = (*Pty)(nil)

// Prompt types text plus Enter and returns once Turn moved past its value
// before, nudging and retyping through claudeui.PromptWithNudge.
func (p *Pty) Prompt(s agentrunner.Session, text string) error {
	return p.PromptNudged(s, text, nil)
}

func (p *Pty) PromptNudged(s agentrunner.Session, text string, onNudge func(string)) error {
	ss, err := p.reg.live(s)
	if err != nil {
		return err
	}
	turn, err := p.turns().awaitNotBusy(ss)
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
		err = p.turns().promptSlash(pr, text)
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

// Interrupt picks its key by state: Ctrl+C when busy, a lone Esc when blocked
// (it denies the dialog), nothing otherwise. After a key it waits up to N for
// idle, then presses Ctrl+C once: claude restores the cancelled prompt into the
// input line, and it would prefix the next prompt. Ctrl+U clears one line per
// press, so it can't empty a multi-line prompt; Ctrl+C on an idle prompt clears
// it all. It is sent once and only after idle, since a second quick Ctrl+C
// quits claude. The session is kept, so smart-zone can send /compact next with
// no further wait.
func (p *Pty) Interrupt(s agentrunner.Session) error {
	ss, err := p.reg.live(s)
	if err != nil {
		return err
	}
	resp, err := ss.holder.StatusReply()
	if err != nil {
		return p.died(ss, err)
	}
	claude, program := resp.Claude, resp.Program
	w := resp.WatchedView()
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
	if err := p.turns().awaitIdle(ss); err != nil {
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

// Answer answers the session's dialog; see holderAnswerer.AnswerClaude and
// AnswerCodex.
func (p *Pty) Answer(s agentrunner.Session, a agentrunner.Answer) error {
	ss, err := p.reg.live(s)
	if err != nil {
		return err
	}
	if ss.kind == harness.Codex {
		return p.answerer(ss).AnswerCodex(a)
	}
	return p.answerer(ss).AnswerClaude(a)
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
