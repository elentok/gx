package sched

import (
	"cmp"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/ralphloop/supervise"
	"github.com/elentok/gx/ralphloop/supervise/startprompt"
	"github.com/elentok/gx/ralphloop/ticketops"
	"github.com/elentok/gx/server/budget"
	"github.com/elentok/gx/server/merge"
	"github.com/elentok/gx/server/queue"
	srvrecovery "github.com/elentok/gx/server/recovery"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/oneoff"
	"github.com/elentok/gx/tickets/ticketlog"
)

// claimed is a ticket whose claim is on disk: the serial half is done and the
// launch half is left.
type claimed struct {
	root      queue.RootRef
	queued    tickets.Address
	ticket    tickets.Address
	t         tickets.Ticket
	dir       string
	repo      string
	agentKind agent.AgentKind
	epics     []tickets.Epic
	rootBase  string
	leafBase  string
	answered  string
	entry     runs.TrackedRun
}

// claim resolves the base and writes the claim. A nil claimed with a nil error
// means the ticket paused on an ambiguous base. The claim loop calls it under
// Guards.AdoptMu, and only this half: the rest runs outside the lock.
func (l *Launcher) claim(root queue.RootRef, queued tickets.Address, t tickets.Ticket, repo string, agentKind agent.AgentKind, epics []tickets.Epic) (*claimed, error) {
	ticket := tickets.Address{Project: queued.Project, Epic: queued.Epic, ID: t.Identifier}
	ticketAddr := ticket.String()
	// The project dir, not the store root: the run log, merge lock and
	// conflict-resolution tickets all live under <project>/<epic>.
	dir, err := l.Projects.Dir(queued.Project)
	if err != nil {
		return nil, fmt.Errorf("claim %s: %w", ticketAddr, err)
	}
	rootBase, resolvedBase, leafBase, err := l.Host.ResolveBase(queued, t, repo, epics)
	var ambiguous *tickets.AmbiguousBaseError
	if errors.As(err, &ambiguous) {
		reason := "Ambiguous base: choose which of " + strings.Join(ambiguous.Blockers, ", ") + " this ticket starts from, by setting base:."
		if perr := l.Writer.Pause(dir, ticket, t.Path, events.AmbiguousBase, reason, runs.Run{}); perr != nil {
			return nil, fmt.Errorf("pause %s: %w", ticketAddr, perr)
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("base for %s: %w", ticketAddr, err)
	}
	answered := AnsweredSession(t)
	if err := ticketops.ClaimWithBase(t.Path, resolvedBase); err != nil {
		return nil, fmt.Errorf("claim %s: %w", ticketAddr, err)
	}
	return &claimed{
		root: root, queued: queued, ticket: ticket, t: t, dir: dir, repo: repo, agentKind: agentKind, epics: epics,
		rootBase: rootBase, leafBase: leafBase, answered: answered,
		entry: runs.TrackedRun{
			// The runner is named now: a reservation with none reads as herdr to the run watchers.
			Run:  runs.Run{Address: ticketAddr, Agent: string(agentKind), Runner: l.Harness.LaunchRunner(ticket.Project, agentKind).Name()},
			Root: root.String(), Repo: repo, TicketPath: t.Path, StartedAt: time.Now(), State: runs.Launching, LaunchStep: runs.StepCreatingWorktree,
		},
	}, nil
}

// reserve shows the claim in the index and takes the slot, so the caps count
// the launch before it runs. False means a stale scan offered a ticket that is
// already running.
func (l *Launcher) reserve(c *claimed) bool {
	ticketAddr := c.entry.Address
	// Refresh, not Rescan: a launch never runs an OnRescan step.
	if err := l.Index.Refresh(); err != nil {
		l.Log.Warn("refresh index after claim", "ticket", ticketAddr, "err", err)
	}
	l.Events.Publish(wire.EventTicketClaimed, ticketAddr)
	return l.Registry.PutIfAbsent(c.entry)
}

// goLaunch runs the launch half of a reserved claim on its own tracked
// goroutine. A launch that ends without an agent frees its slot, so the claim
// loop is kicked to refill it.
func (l *Launcher) goLaunch(c *claimed) {
	l.launching.Go(func() {
		launched, err := l.launchClaimed(c)
		if err != nil {
			l.Log.Warn("launch", "ticket", c.entry.Address, "err", err)
		}
		if !launched {
			l.Guards.Kick()
		}
	})
}

// WaitLaunches waits for the launch goroutines the claim loop started. Serve
// calls it at the stop, after the watches close cancel every launch's context.
func (l *Launcher) WaitLaunches() { l.launching.Wait() }

// handBackStopped gives a launch the server's stop cut off its claim back, as
// reclaim does for an empty claim after a restart. Its worktree and agent are
// already gone.
func (l *Launcher) handBackStopped(ticket tickets.Address, ticketPath string) {
	const reason = "server stopped during launch"
	if err := ticketlog.StartOver(ticketPath, time.Now(), reason, ""); err != nil {
		l.Log.Warn("hand back stopped launch", "ticket", ticket.String(), "err", err)
		return
	}
	l.LogEvent(ticket, runlog.Event{Type: string(events.ClaimReopened), Ticket: ticket.ID, Address: ticket.String(), Reason: reason})
	l.Events.Publish(wire.EventTicketChanged, ticket.String())
}

// launchClaimed is the launch half: model choice, the worktree and agent, and
// the registry updates. It owns c's reservation and gives it back on any end
// that leaves no agent.
func (l *Launcher) launchClaimed(c *claimed) (launched bool, err error) {
	root, queued, ticket, t, dir, repo, agentKind, epics := c.root, c.queued, c.ticket, c.t, c.dir, c.repo, c.agentKind, c.epics
	ticketAddr, entry := c.entry.Address, c.entry
	defer func() {
		if !launched {
			l.Registry.DeleteStarted(ticketAddr, entry.StartedAt)
		}
	}()
	if err := budget.ClearLongTicketMark(t.Path); err != nil {
		l.Log.Warn("clear long-ticket mark", "ticket", ticketAddr, "err", err)
	}
	model, effort, ok := l.Writer.LaunchModel(dir, t, ticket, agentKind)
	if !ok {
		return false, nil
	}
	answered := c.answered
	// Cancelled at the server's stop: it cuts the install and the prompt wait short.
	ctx, release := l.Sessions.Watches.Context()
	defer release()
	one := ralphloop.OneIteration{
		Context: ctx,
		RepoDir: repo, Epic: queued.Epic, ScratchDir: dir, Agent: agentKind, Ticket: t, RootBase: c.rootBase, LeafBase: c.leafBase,
		Model: model, Effort: effort, Sink: l.Writer.FinishSink(ticket, t.Path), StallNudgeDelay: l.Host.StalledNudgeDelay(),
		OnDepsStart: func() { l.setLaunchStep(ticketAddr, runs.StepInstallingDeps) },
		OnDepsDone:  func() { l.setLaunchStep(ticketAddr, runs.StepStartingAgent) },
	}
	deps := l.Projects.DepsWith(l.Harness.LaunchRunner(ticket.Project, agentKind))
	deps.GateReleased = l.Registry.GateReleased(ticketAddr)
	// Only Claude and Codex on the pty runner resume; every other claim starts
	// cold, and so does one under another harness than the answered session's:
	// the Handoff carries it over. launch drops a Codex id whose rollout is gone.
	earlier := l.Harness.EarlierRunHarness(ticket)
	switchedHarness := earlier != "" && earlier != string(agentKind)
	if canResume(agentKind, deps.Runner) && !switchedHarness {
		one.ResumeSessionID = answered
	}
	mode := l.Landing.IterationModeFor(ticket, t)
	if mode.Commitless && mode.ScratchDir == "" {
		if mode.Ref, err = l.Host.CommitlessRef(ticket, t, repo, epics); err != nil {
			return false, fmt.Errorf("base for %s: %w", ticketAddr, err)
		}
	}
	if mode.Feature {
		if one.TargetTip, err = l.Host.MergeTargetTip(ticket, repo); err != nil {
			return false, fmt.Errorf("merge target for %s: %w", ticketAddr, err)
		}
	}
	entry.Runner, entry.Model, entry.Effort = deps.Runner.Name(), one.Model, one.Effort
	run, wt, err := l.prepareAndLaunch(deps, &one, ticket, mode, func(wt ralphloop.IterationWorktree) {
		entry.Workspace, entry.Base, entry.Cwd = one.WorkspaceID, wt.Base(), wt.Path
		l.Registry.Replace(entry)
	})
	if err != nil {
		l.Registry.DeleteStarted(ticketAddr, entry.StartedAt)
		if ctx.Err() != nil {
			// A stop is not a failure: nothing pauses and nothing is published.
			l.handBackStopped(ticket, t.Path)
			return false, nil
		}
		// Handing the claim back would make the next tick retry the same failure
		// forever, so a person is told instead.
		kind, reason := events.IterationError, "launch failed: "+err.Error()
		if taken := (*nameTakenError)(nil); errors.As(err, &taken) {
			kind, reason = events.AgentNameTaken, "agent name taken: "+taken.reason
		} else if k, ok := startprompt.LaunchFailureKind(err); ok {
			kind = k
		}
		if kind == events.IterationError {
			reason = markedIterationReason(err, reason)
		}
		if perr := l.Writer.Pause(dir, ticket, t.Path, kind, reason, entry.Run); perr != nil {
			err = errors.Join(err, fmt.Errorf("pause: %w", perr))
		}
		l.Events.Publish(wire.EventIterationLaunchFailed, ticketAddr)
		return false, fmt.Errorf("launch %s: %w", ticketAddr, err)
	}
	// The launch context ends with this function; the settle path outlives it.
	one.Context = nil
	l.Registry.Replace(runs.TrackedRun{
		Run: run, Root: root.String(), Repo: repo, Workspace: one.WorkspaceID, Base: wt.Base(), Cwd: wt.Path, TicketPath: t.Path, StartedAt: entry.StartedAt,
	})
	l.Events.Publish(wire.EventIterationStarted, ticketAddr)
	l.Guards.GoSettle(func() { l.finishRun(runContext{deps, root, mode, one, wt, run, ticketAddr}) })
	return true, nil
}

// Continue hands a reclaimed run to the settle machinery: settle now when
// settleNow, else wait for the agent to finish first.
func (l *Launcher) Continue(runner agentrunner.Runner, t runs.TrackedRun, root queue.RootRef, mode merge.IterationMode, one ralphloop.OneIteration, wt ralphloop.IterationWorktree, settleNow bool) {
	deps := l.Projects.DepsWith(runner)
	deps.GateReleased = l.Registry.GateReleased(t.Address)
	rc := runContext{deps, root, mode, one, wt, t.Run, t.Address}
	if settleNow {
		l.Guards.GoSettle(func() { l.settleRun(rc, nil) })
		return
	}
	l.Guards.GoSettle(func() { l.finishRun(rc) })
}

// prepareAndLaunch gives the ticket somewhere to run, then launches the agent
// in it. A launch that fails takes that back out so a retry starts clean.
// prepared sees the worktree before the agent starts.
func (l *Launcher) prepareAndLaunch(
	deps deps.Deps, one *ralphloop.OneIteration, ticket tickets.Address, mode merge.IterationMode,
	prepared func(ralphloop.IterationWorktree),
) (runs.Run, ralphloop.IterationWorktree, error) {
	wt, err := mode.Prepare(deps, *one)
	if err != nil {
		return runs.Run{}, ralphloop.IterationWorktree{}, err
	}
	prepared(wt)
	run, err := l.launch(deps, ticket, *one, wt.Path)
	if err != nil {
		if derr := mode.Discard(deps, *one, wt); derr != nil {
			err = errors.Join(err, fmt.Errorf("discard worktree: %w", derr))
		}
		return runs.Run{}, ralphloop.IterationWorktree{}, err
	}
	return run, wt, nil
}

// finishRun waits for the agent to settle, then merges (or pauses) the ticket and
// moves the root on: the next frontier ticket, or the root's completion.
func (l *Launcher) finishRun(rc runContext) {
	rc.one.Runner = rc.run.Runner
	rc.one.OnRateLimit = func(limited bool, resetAt time.Time) {
		l.Registry.MarkRateLimit(rc.ticketAddr, limited, resetAt)
	}
	addr, _ := tickets.ParseAddress(rc.ticketAddr, tickets.AddressContext{}) // built by ClaimAndLaunch, always parses
	l.Budget.GuardCallbacks(addr, &rc.one)
	stop := make(chan struct{})
	if !rc.one.PausesAtOnce() {
		go l.watchBlocked(rc.deps.Runner, rc.run.Session, rc.ticketAddr, stop)
	}
	err := l.finishWithRecovery(&rc, addr, func() { close(stop) })
	l.settleRun(rc, err)
}

// finishWithRecovery waits for the run, relaunching a hung pane once and then a
// dead holder once. stopWatch runs after the hung ladder, before the dead-holder
// relaunch. rc follows any relaunch.
func (l *Launcher) finishWithRecovery(rc *runContext, addr tickets.Address, stopWatch func()) error {
	err := l.waitKeepingHolder(*rc, addr)
	if _, hung := errors.AsType[*ralphloop.RelaunchRequest](err); hung {
		err = l.relaunchHung(rc, err)
		if err == nil {
			err = l.waitKeepingHolder(*rc, addr)
			if _, again := errors.AsType[*ralphloop.RelaunchRequest](err); again {
				l.stopHung(*rc)
				err = &startprompt.LaunchFailure{Kind: events.AgentPromptStalled, Err: fmt.Errorf("pane stopped taking keys, twice: %w", err)}
			}
		}
	}
	stopWatch()
	if errors.Is(err, agentrunner.ErrHolderDead) {
		// One relaunch through the shared launch path before any pause.
		if rerr := l.relaunchDeadHolder(rc); rerr != nil {
			err = fmt.Errorf("holder dead (%v); relaunch failed: %w", err, rerr)
		} else {
			err = l.waitKeepingHolder(*rc, addr)
		}
	}
	return err
}

// canResume reports whether a launch under kind on runner can resume a session:
// only Claude and Codex on the pty runner do; every other launch starts cold.
func canResume(kind agent.AgentKind, runner agentrunner.Runner) bool {
	return (kind == agent.AgentClaude || kind == agent.AgentCodex) && runner.Name() == string(agentrunner.ChoicePTY)
}

// relaunchDeadHolder stops what is left of the run's session and launches the
// agent again in the same worktree, resuming the session so its conversation
// (and the ticket's recorded verdicts, which live in the ticket body) carry on.
func (l *Launcher) relaunchDeadHolder(rc *runContext) error {
	resumeID := ""
	if canResume(rc.one.Agent, rc.deps.Runner) {
		resumeID = rc.run.Session.SessionID
	}
	return l.relaunch(rc, resumeID, nil)
}

// HolderReadoptCap is how many unanswered dials in a row a run with a live
// holder takes before it pauses.
const HolderReadoptCap = 5

// waitKeepingHolder is WaitIterationFinished that survives a holder which is
// alive but does not answer: each tick it probes the session again (the
// re-adopt) and resumes the wait once a dial works. The run stays tracked and
// is never relaunched; only the cap ends it.
func (l *Launcher) waitKeepingHolder(rc runContext, addr tickets.Address) error {
	failures := 0 // above zero, the loop is probing the session instead of waiting
	holder.ArmUnreachableStatusDials()
	for {
		var err error
		if failures == 0 {
			err = ralphloop.WaitIterationFinished(rc.deps, rc.one, rc.wt, rc.run.Session)
			if !errors.Is(err, holder.ErrHolderUnreachable) {
				return err
			}
		} else {
			time.Sleep(cmp.Or(rc.deps.GuardPoll, supervise.SmartZonePollMs*time.Millisecond))
			_, err = rc.deps.Runner.Status(rc.run.Session)
			if !errors.Is(err, holder.ErrHolderUnreachable) {
				// The probe answered, or found the session gone: the wait reports that.
				l.markHolderUnreachable(addr, false)
				failures = 0
				continue
			}
		}
		failures++
		if failures >= HolderReadoptCap {
			l.markHolderUnreachable(addr, false)
			return fmt.Errorf("dial failed %d times with the holder alive: %w", failures, err)
		}
		l.markHolderUnreachable(addr, true)
	}
}

// markHolderUnreachable sets or clears the card's retrying state, logging the
// event once per hold.
func (l *Launcher) markHolderUnreachable(addr tickets.Address, unreachable bool) {
	if !l.Registry.MarkHolderUnreachable(addr.String(), unreachable) {
		return
	}
	if unreachable {
		l.LogEvent(addr, runlog.Event{Type: string(events.HolderUnreachable), Ticket: addr.ID, Address: addr.String(), Reason: "holder unreachable, retrying"})
	}
	l.Events.Publish(wire.EventTicketChanged, addr.String())
}

// relaunchHung replaces the run's hung session with a fresh launch, once. The
// old session must be gone before the new one starts: never two agents on one
// ticket.
func (l *Launcher) relaunchHung(rc *runContext, hung error) error {
	return l.relaunch(rc, rc.one.ResumeSessionID, func() error {
		if _, live, _ := rc.deps.Runner.Find(rc.run.Session.Label); live {
			return &startprompt.LaunchFailure{Kind: events.AgentPromptStalled, Err: fmt.Errorf("%w: %s is still live after stop, not relaunching", hung, rc.run.Session.Label)}
		}
		return nil
	})
}

// relaunch stops the run's session and launches the agent again in the same
// worktree, resuming resumeID when set. afterStop, when non-nil, may veto the
// launch. On success rc.run, rc.one and the registry follow the new session; on
// failure rc is unchanged.
func (l *Launcher) relaunch(rc *runContext, resumeID string, afterStop func() error) error {
	if err := rc.deps.Runner.Stop(rc.run.Session); err != nil {
		l.Log.Warn("stop session before relaunch", "ticket", rc.ticketAddr, "err", err)
	}
	if afterStop != nil {
		if err := afterStop(); err != nil {
			return err
		}
	}
	addr, _ := tickets.ParseAddress(rc.ticketAddr, tickets.AddressContext{}) // built by ClaimAndLaunch, always parses
	// Cancelled at the server's stop, like the first launch.
	ctx, release := l.Sessions.Watches.Context()
	defer release()
	one := rc.one
	one.Context = ctx
	one.ResumeSessionID = resumeID
	run, err := l.launch(rc.deps, addr, one, rc.wt.Path)
	if err != nil {
		return err
	}
	one.Context = nil
	if t, ok := l.Registry.Tracked(rc.ticketAddr); ok {
		t.Run = run
		l.Registry.Replace(t)
	}
	l.Events.Publish(wire.EventTicketChanged, rc.ticketAddr)
	rc.one, rc.run = one, run
	return nil
}

// stopHung closes a session that hung a second time.
func (l *Launcher) stopHung(rc runContext) {
	if err := rc.deps.Runner.Stop(rc.run.Session); err != nil {
		l.Log.Warn("stop twice-hung session", "ticket", rc.ticketAddr, "err", err)
	}
}

// BlockWatchPoll bounds each Wait of watchBlocked. A herdr Wait is one herdr
// process, so a short poll would cost an idle server a process per tick.
const BlockWatchPoll = 30 * time.Second

// watchBlocked publishes a ticket change each time the agent enters or leaves
// a blocked state, until stop closes or the session ends. The settle wait
// never returns on a block (a blocked iteration stays running), and the
// permission card is read only when a page fetches, so without this an open
// page never shows the prompt.
func (l *Launcher) watchBlocked(r agentrunner.Runner, sess agentrunner.Session, addr string, stop <-chan struct{}) {
	blocked := false
	for {
		until := []agentrunner.State{agentrunner.StateBlocked}
		if blocked {
			until = []agentrunner.State{agentrunner.StateWorking, agentrunner.StateIdle, agentrunner.StateDone}
		}
		_, err := r.Wait(sess, until, BlockWatchPoll)
		select {
		case <-stop:
			return
		default:
		}
		if errors.Is(err, agentrunner.ErrTimeout) {
			continue
		}
		if err != nil {
			return
		}
		blocked = !blocked
		l.Events.Publish(wire.EventTicketChanged, addr)
	}
}

// runContext is everything settling one claimed iteration needs.
type runContext struct {
	deps       deps.Deps
	root       queue.RootRef
	mode       merge.IterationMode
	one        ralphloop.OneIteration
	wt         ralphloop.IterationWorktree
	run        runs.Run
	ticketAddr string
}

// settleRun merges (or pauses) an iteration whose wait ended with waitErr. A run
// found already finished at reattach skips the wait and comes straight here.
func (l *Launcher) settleRun(rc runContext, err error) {
	deps, root, mode, one, wt, run, ticketAddr := rc.deps, rc.root, rc.mode, rc.one, rc.wt, rc.run, rc.ticketAddr
	defer l.Guards.Kick()
	l.Registry.MarkSettling(ticketAddr)
	keepRun := false
	defer func() {
		if !keepRun {
			l.Registry.Delete(ticketAddr)
			// The sweep skips a tracked run, so this is the earliest it can archive it.
			if a, err := tickets.ParseAddress(ticketAddr, tickets.AddressContext{}); err == nil && a.Epic == queue.OneOffsEpic {
				l.Index.Rescan()
			}
		}
	}()
	// A run the budget hard limit is stopping never finishes, merges or pauses
	// for its own reason: the first of it and the kill pauses it budget-killed.
	if t, ok := l.Registry.Tracked(ticketAddr); ok {
		stopping, perr := l.Budget.Stopper().SettleStop(t)
		if perr != nil {
			l.Log.Error("stop pause", "ticket", ticketAddr, "err", perr)
		}
		if stopping {
			return
		}
	}
	addr, _ := tickets.ParseAddress(ticketAddr, tickets.AddressContext{}) // built by ClaimAndLaunch, always parses
	if l.Recovery.PauseReleasedGate(one.ScratchDir, addr, one.Ticket.Path, run) {
		return
	}
	var blocked *supervise.BlockedPausedError
	if errors.As(err, &blocked) {
		// The wait already wrote the pause. The agent is alive and waits for a
		// person: hand the run to the waiting-run machinery as a question does.
		keepRun = true
		l.Registry.MarkWaiting(ticketAddr, time.Now())
		l.Events.Publish(wire.EventIterationPaused, ticketAddr)
		l.Writer.AnnouncePause(addr, one.Ticket.Path, events.BlockedPane, blocked.Reason)
		go l.Writer.WatchUnblock(deps.Runner, run.Session, ticketAddr)
		return
	}
	var mismatch *supervise.ModelMismatchError
	if errors.As(err, &mismatch) {
		l.Writer.PauseModelMismatch(rc.one.ScratchDir, rc.one.Ticket.Path, addr, rc.run, mismatch)
		return
	}
	// Best effort: a missing summary never blocks finishing or pauses the ticket.
	if serr := srvrecovery.RecordGxSummary(one.ScratchDir, addr, one.Ticket.Path, run.Session.SessionID, time.Now()); serr != nil {
		l.Log.Warn("write gx iteration summary", "ticket", ticketAddr, "err", serr)
	}
	var out ralphloop.FinishOutcome
	if err == nil {
		// A stop that began while the agent settled leaves it for the next
		// server, which reclaims it from the saved registry: keep it there.
		if !l.Guards.Merges.Begin() {
			keepRun = true
			return
		}
		defer l.Guards.Merges.End()
		one.Model, one.Effort = l.Projects.RunAgentConfig(addr.Project, one.Agent, run)
		one.KeepAskedAgent = !l.DropAskedAgent && keepsAskedAgent(run)
		out, err = mode.Finish(deps, one, wt, run.Session.ID)
		// The finish path wrote the merge or the pause; the store watch is only a hint.
		l.Index.Rescan()
	}
	if err != nil {
		l.Log.Warn("finish iteration", "ticket", ticketAddr, "err", err)
		// Pause it: a claimed ticket nobody is working on is stuck, not failed.
		kind, reason := events.IterationError, markedIterationReason(err, err.Error())
		if k, ok := startprompt.LaunchFailureKind(err); ok {
			kind, reason = k, err.Error()
		}
		if perr := l.Writer.Pause(one.ScratchDir, addr, one.Ticket.Path, kind, reason, run); perr != nil {
			l.Log.Warn("pause failed finish", "ticket", ticketAddr, "err", perr)
		}
		l.Events.Publish(wire.EventIterationFailed, ticketAddr)
		return
	}
	if out.Cancelled {
		// The finish check closed the attempt and the next step is already written.
		if cerr := agentrunner.Cleanup(deps.Runner, run.Session.Label); cerr != nil {
			l.Log.Warn("clean up agent", "ticket", ticketAddr, "err", cerr)
		}
		l.Log.Info("attempt cancelled", "ticket", ticketAddr, "reason", out.Reason)
		return
	}
	if !out.Merged {
		// The finish path already wrote the pause; tell a person its own reason.
		if one.KeepAskedAgent && out.Kind == events.SelfReported {
			// The agent waits idle for the answer: its run, worktree and session stay.
			keepRun = true
			l.Registry.MarkWaiting(ticketAddr, time.Now())
		}
		l.Events.Publish(wire.EventIterationPaused, ticketAddr)
		l.Writer.AnnouncePause(addr, one.Ticket.Path, out.Kind, out.Reason)
		return
	}
	// Stop already cleans an adopted session; a run found finished at reattach
	// was never adopted, so its I/O files are removed here.
	if cerr := agentrunner.Cleanup(deps.Runner, run.Session.Label); cerr != nil {
		l.Log.Warn("clean up agent", "ticket", ticketAddr, "err", cerr)
	}
	l.Events.Publish(wire.EventTicketDone, ticketAddr)
	l.Writer.StampFinished(addr, one.Ticket.Path, false)
	l.OneOffs.NotifyResult(addr, one.Ticket.Path)
	l.NotifyDone(addr, one.Ticket.Path)
	l.Recovery.FileFollowUp(addr, one.Ticket.Path)
	if mode.AfterDone != nil {
		mode.AfterDone()
	}
	l.Merger.CompleteRootIfDone(root, one)
}

// keepsAskedAgent reports whether run's agent stays alive after it asks: a
// Claude agent on a native runner, or a Codex agent on the pty runner.
// Herdr-hosted runs clean up as before.
func keepsAskedAgent(run runs.Run) bool {
	switch run.Agent {
	case string(agent.AgentClaude):
		return run.Runner != "" && run.Runner != runs.RunnerHerdr
	case string(agent.AgentCodex):
		return run.Runner == string(agentrunner.ChoicePTY)
	}
	return false
}

// NotifyDone hands a merged ticket to the epic's next Progress message; it
// sends nothing itself. A one-off never counts: it speaks only through
// notifyResult, when its submit asked for it. Exported for tests.
func (l *Launcher) NotifyDone(addr tickets.Address, ticketPath string) {
	if !l.Chat.Enabled() || oneoff.IsOneOffTicket(ticketPath) {
		return
	}
	if e, t, ok := l.Writer.FreshTicket(addr); ok && !oneoff.IsOneOffEpic(e) {
		l.Progress.Finished(addr, t)
	}
}
