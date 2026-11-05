package sched

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/budget"
	"github.com/elentok/gx/server/merge"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// ClaimAndLaunch writes the claim to the ticket file first, so markdown stays
// the truth and nothing streams before it is on disk. A failed launch pauses the
// ticket stuck rather than leaving a claim with no agent behind it.
// launched is false when the ticket paused on an ambiguous base: no agent runs.
// epics is the project's loaded epics, for the base derivation.
func (l *Launcher) ClaimAndLaunch(root queue.RootRef, queued tickets.Address, t tickets.Ticket, repo string, agentKind agent.AgentKind, epics []tickets.Epic) (launched bool, err error) {
	ticket := tickets.Address{Project: queued.Project, Epic: queued.Epic, ID: t.Identifier}
	ticketAddr := ticket.String()
	// The project dir, not the store root: the run log, merge lock and
	// conflict-resolution tickets all live under <project>/<epic>.
	dir, err := l.Projects.Dir(queued.Project)
	if err != nil {
		return false, fmt.Errorf("claim %s: %w", ticketAddr, err)
	}
	rootBase, resolvedBase, leafBase, err := l.Host.ResolveBase(queued, t, repo, epics)
	var ambiguous *tickets.AmbiguousBaseError
	if errors.As(err, &ambiguous) {
		reason := "Ambiguous base: choose which of " + strings.Join(ambiguous.Blockers, ", ") + " this ticket starts from, by setting base:."
		if perr := l.Writer.Pause(dir, ticket, t.Path, events.AmbiguousBase, reason, runs.Run{}); perr != nil {
			return false, fmt.Errorf("pause %s: %w", ticketAddr, perr)
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("base for %s: %w", ticketAddr, err)
	}
	answered := AnsweredSession(t)
	if err := ralphloop.ClaimWithBase(t.Path, resolvedBase); err != nil {
		return false, fmt.Errorf("claim %s: %w", ticketAddr, err)
	}
	l.Index.Rescan() // the claim must show now, not when the backstop poll comes
	l.Events.Publish(wire.EventTicketClaimed, ticketAddr)

	if err := budget.ClearLongTicketMark(t.Path); err != nil {
		l.Log.Warn("clear long-ticket mark", "ticket", ticketAddr, "err", err)
	}
	model, effort, ok := l.Writer.LaunchModel(dir, t, ticket, agentKind)
	if !ok {
		return false, nil
	}
	one := ralphloop.OneIteration{
		RepoDir: repo, Epic: queued.Epic, ScratchDir: dir, Agent: agentKind, Ticket: t, RootBase: rootBase, LeafBase: leafBase,
		Model: model, Effort: effort, Sink: l.Writer.FinishSink(ticket, t.Path), StallNudgeDelay: l.Host.StalledNudgeDelay(),
	}
	deps := l.Projects.DepsWith(l.Harness.LaunchRunner(ticket.Project, agentKind))
	deps.GateReleased = l.Registry.GateReleased(ticketAddr)
	// Only Claude and Codex on the pty runner resume; every other claim starts
	// cold, and so does one under another harness than the answered session's:
	// the Handoff carries it over. launch drops a Codex id whose rollout is gone.
	earlier := l.Harness.EarlierRunHarness(ticket)
	switchedHarness := earlier != "" && earlier != string(agentKind)
	if (agentKind == agent.AgentClaude || agentKind == agent.AgentCodex) && deps.Runner.Name() == string(agentrunner.ChoicePTY) && !switchedHarness {
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
	entry := runs.TrackedRun{
		Run:  runs.Run{Address: ticketAddr, Agent: string(agentKind), Runner: deps.Runner.Name(), Model: one.Model, Effort: one.Effort},
		Root: root.String(), Repo: repo, TicketPath: t.Path, StartedAt: time.Now(), State: runs.Launching,
	}
	if !l.Registry.PutIfAbsent(entry) {
		return false, nil // a stale scan offered a ticket that is already running
	}
	run, wt, err := l.prepareAndLaunch(deps, &one, ticket, mode, func(wt ralphloop.IterationWorktree) {
		entry.Workspace, entry.Base, entry.Cwd = one.WorkspaceID, wt.Base(), wt.Path
		l.Registry.Put(entry)
	})
	if err != nil {
		l.Registry.DeleteStarted(ticketAddr, entry.StartedAt)
		// Handing the claim back would make the next tick retry the same failure
		// forever, so a person is told instead.
		kind, reason := events.IterationError, "launch failed: "+err.Error()
		if taken := (*nameTakenError)(nil); errors.As(err, &taken) {
			kind, reason = events.AgentNameTaken, "agent name taken: "+taken.reason
		} else if k, ok := ralphloop.LaunchFailureKind(err); ok {
			kind = k
		}
		if perr := l.Writer.Pause(dir, ticket, t.Path, kind, reason, entry.Run); perr != nil {
			err = errors.Join(err, fmt.Errorf("pause: %w", perr))
		}
		l.Events.Publish(wire.EventIterationLaunchFailed, ticketAddr)
		return false, fmt.Errorf("launch %s: %w", ticketAddr, err)
	}
	l.Registry.Put(runs.TrackedRun{
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
	deps ralphloop.Deps, one *ralphloop.OneIteration, ticket tickets.Address, mode merge.IterationMode,
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
	err := ralphloop.WaitIterationFinished(rc.deps, rc.one, rc.wt, rc.run.Session)
	close(stop)
	l.settleRun(rc, err)
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
	deps       ralphloop.Deps
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
	if stopping, first := l.Registry.ClaimBudgetStop(ticketAddr); stopping {
		if t, ok := l.Registry.Tracked(ticketAddr); ok && first {
			if perr := l.Budget.PauseKilled(t); perr != nil {
				l.Log.Error("budget kill", "ticket", ticketAddr, "err", perr)
			}
		}
		return
	}
	addr, _ := tickets.ParseAddress(ticketAddr, tickets.AddressContext{}) // built by ClaimAndLaunch, always parses
	if l.Recovery.PauseReleasedGate(one.ScratchDir, addr, one.Ticket.Path, run) {
		return
	}
	var blocked *ralphloop.BlockedPausedError
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
	var mismatch *ralphloop.ModelMismatchError
	if errors.As(err, &mismatch) {
		l.Writer.PauseModelMismatch(rc.one.ScratchDir, rc.one.Ticket.Path, addr, rc.run, mismatch)
		return
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
		if perr := l.Writer.Pause(one.ScratchDir, addr, one.Ticket.Path, events.IterationError, err.Error(), run); perr != nil {
			l.Log.Warn("pause failed finish", "ticket", ticketAddr, "err", perr)
		}
		l.Events.Publish(wire.EventIterationFailed, ticketAddr)
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

// NotifyDone tells chat a ticket merged. A one-off stays silent: it speaks
// only through notifyResult, when its submit asked for it. Exported for tests.
func (l *Launcher) NotifyDone(addr tickets.Address, ticketPath string) {
	if !l.Chat.Enabled() || queue.IsOneOffTicket(ticketPath) {
		return
	}
	if e, t, ok := l.Writer.FreshTicket(addr); ok && !queue.IsOneOffEpic(e) {
		l.Chat.TicketDone(addr.Project, l.Projects.ChatOverride(addr.Project), addr.Epic, t, ralphloop.CountsOf(e))
	}
}
