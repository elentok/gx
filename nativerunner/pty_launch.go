package nativerunner

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/transcript"
)

var _ agentrunner.LaunchPrompter = (*Pty)(nil)

// PromptLaunch is PromptNudged for a launch prompt, which must start a turn.
// A Claude "/" command is typed once and then proved by the transcript: its
// first user record must be the command (see claudeui.CheckLaunchDelivered,
// which also owns the resends). A person typing over it is unconfirmed unless
// the transcript already shows the command. Other prompts, Codex, and runners
// without a Claude home keep the PromptNudged path.
func (p *Pty) PromptLaunch(ctx context.Context, s agentrunner.Session, text string, onNudge func(string)) error {
	home := envValue(p.Env, "HOME")
	ss, err := p.reg.live(s)
	if err != nil {
		return err
	}
	if home == "" || (ss.kind != "" && ss.kind != harness.Claude) || !isSlashCommand(text) {
		return p.PromptNudged(s, text, onNudge)
	}
	turn, err := p.awaitNotBusy(ss)
	if err != nil {
		return err
	}
	pr := &ptyPrompter{p: p, ctx: ctx, ss: ss, turn: turn}
	if person, stop, err := p.PersonInput(s); err == nil {
		defer stop()
		pr.person = person
	}
	if err := pr.awaitReady(ctx, home); err != nil {
		return err
	}
	if err := pr.awaitEmptyInput(p.promptGrace()); err != nil {
		return notDelivered(err)
	}
	if err := pr.Submit(text); err != nil {
		return notDelivered(err)
	}
	proof := claudeui.LaunchProof{
		FirstUser: func() (string, bool, error) { return claudeui.FirstUserRecord(home, s.SessionID) },
		Pane: func() (string, error) {
			if err := pr.cancelled(); err != nil {
				return "", err
			}
			return pr.Screen()
		},
		Resend: func() error { return pr.Submit(text) },
		Report: func(d claudeui.NudgeDecision) {
			if onNudge != nil {
				onNudge(string(d))
			}
		},
		Now:   time.Now,
		Sleep: time.Sleep,
	}
	err = claudeui.CheckLaunchDelivered(ctx, proof, text, p.launchCaps())
	if errors.Is(err, errPersonInput) {
		// The person's keys end the resends; the transcript may still show the
		// command from before they typed.
		if ok, rerr := claudeui.LaunchRecorded(home, s.SessionID, text); rerr != nil || !ok {
			return fmt.Errorf("%w: %q: a person typed before the transcript showed the command", agentrunner.ErrDeliveryUnconfirmed, text)
		}
		return nil
	}
	if err != nil {
		return err
	}
	// The command is in the transcript; a launch prompt must also start a turn.
	started, err := pr.WaitStart(p.promptGrace())
	switch {
	case errors.Is(err, errPersonInput):
		return nil
	case err != nil:
		return err
	case !started:
		return fmt.Errorf("%w: %q started no turn", agentrunner.ErrNotDelivered, text)
	}
	return nil
}

// DefaultReadyWait caps the wait for a launch prompt's ready signal.
const DefaultReadyWait = 30 * time.Second

// awaitReady waits for claude's ready signal: the input box on screen and the
// session transcript on disk. A missing signal is logged and the prompt goes
// anyway: the resend loop in CheckLaunchDelivered backstops a wrong signal, and
// a claude that never shows it must still get its prompt. A cancelled ctx
// returns at once with an error wrapping ctx.Err(), never the warning path.
func (pr *ptyPrompter) awaitReady(ctx context.Context, home string) error {
	p := pr.p
	wait := cmp.Or(p.ReadyWait, DefaultReadyWait)
	deadline := time.Now().Add(wait)
	rows := HolderTypingIO(pr.ss.holder).Rows
	for {
		screen, err := rows()
		if err != nil {
			return p.died(pr.ss, err)
		}
		files, err := transcript.FindByID(home, pr.ss.session.SessionID)
		if err == nil && len(files) == 1 && claudeui.InputBox(screen) {
			return nil
		}
		if !time.Now().Before(deadline) {
			if p.log != nil {
				p.log.Warn("ready signal missing, sending the launch prompt anyway", "session", pr.ss.session.SessionID, "waited", wait)
			}
			return nil
		}
		select {
		case <-pr.ss.exited:
			return p.died(pr.ss, errors.New("holder exited"))
		case <-ctx.Done():
			return fmt.Errorf("launch prompt cancelled: %w", context.Cause(ctx))
		case <-time.After(p.pollInterval()):
		}
	}
}

// launchCaps is the launch proof's caps with LaunchCaps' set fields applied.
func (p *Pty) launchCaps() claudeui.LaunchProofCaps {
	caps := claudeui.DefaultLaunchProofCaps
	caps.Grace = cmp.Or(p.LaunchCaps.Grace, caps.Grace)
	caps.UnknownGrace = cmp.Or(p.LaunchCaps.UnknownGrace, caps.UnknownGrace)
	caps.Poll = cmp.Or(p.LaunchCaps.Poll, p.pollInterval())
	caps.ResendEvery = cmp.Or(p.LaunchCaps.ResendEvery, caps.ResendEvery)
	return caps
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
		if err := p.launchTick(ss, dir, deadline, "holder.sock never answered"); err != nil {
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
		// harness.claudeCommand put editorMode normal on every claude launch.
		EditorModeNormal: kind == harness.Claude,
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
func (p *Pty) launchTick(ss *ptySession, dir string, deadline time.Time, what string) error {
	select {
	case <-ss.exited:
		if text := holderErrText(dir); text != "" {
			return fmt.Errorf("pty start: holder exited: %s", text)
		}
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
	errFile, err := createHolderErr(dir)
	if err != nil {
		return nil, err
	}
	// The child holds its own copy of the file once started.
	defer errFile.Close()
	cmd.Stderr = errFile
	if err := StartDetached(cmd); err != nil {
		return nil, err
	}
	return cmd, nil
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
		env = append(env, harness.CodexSessionEnv(dir)...)
	}
	launchOpts := harness.LaunchOptions{SessionID: sessionID, Args: args, Env: env, Cwd: opts.Cwd}
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
