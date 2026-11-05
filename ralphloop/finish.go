package ralphloop

import (
	"cmp"
	"errors"
	"fmt"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/supervise"
	"github.com/elentok/gx/ralphloop/ticketops"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/metrics"
	"github.com/elentok/gx/ralphloop/runlog"

	"github.com/elentok/gx/ralphloop/worktree"
	"github.com/elentok/gx/tickets/schema"
)

// iterationSession is the live session of p's ticket, for cleanup; the zero
// Session when the iteration has no pane.
func iterationSession(p iterationParams, pane string) agentrunner.Session {
	if pane == "" {
		return agentrunner.Session{}
	}
	return agentrunner.Session{Label: worktree.IterationLabel(p.FeatureBranch, p.Ticket.Identifier), ID: pane}
}

// finishIteration merges a finished iteration's commits (or marks it
// paused (answer) if it produced none), then removes its worktree/tab on success.
// Fresh and reattached iterations both carry their native session and pane
// identity through this shared completion path. A paused (answer) report is
// adopted before any of that: see adoptAskingReport.
func finishIteration(d deps.Deps, p iterationParams, path, pane, base, branch, sessionID string) error {
	adopted, err := adoptAskingReport(p, path, pane, sessionID)
	if err != nil {
		return err
	}
	if adopted && p.KeepAskedAgent {
		return nil
	}
	if adopted {
		// The branch is what makes a later resume able to merge both sides of
		// the answer boundary in one pick (see PrepareIteration's branch-reuse
		// base computation). The worktree stays too, so the relaunch finds the
		// agent's uncommitted work; only the session and tab are redundant
		// while the ticket waits on an answer.
		return stopAskedSession(d, p, pane)
	}

	ahead, err := iterationCommitsAhead(d, path, base, branch)
	if err != nil || ahead == 0 {
		// supervise.WaitForFinish's own debounce (supervise.ConfirmFinished) already guards against
		// herdr reporting the agent idle mid-turn, but a commit can still merge
		// in the gap between that confirmation and this check (e.g. a reattached
		// iteration, which skips supervise.WaitForFinish's launch-time debounce entirely).
		// The count can also fail transiently right after the pane exits (e.g.
		// a worktree/ref momentarily unresolvable during concurrent reconcile
		// activity) even though the agent's work already merged. Recheck once
		// more before giving up rather than orphaning a commit — or stranding
		// an otherwise-finished ticket — on a one-off blip.
		d.Sleep(supervise.FinishDebounceMs * time.Millisecond)
		ahead, err = iterationCommitsAhead(d, path, base, branch)
		if err != nil {
			return fmt.Errorf("counting commits ahead of %s: %w", base, err)
		}
	}
	if ahead == 0 {
		// Re-read the ticket's current frontmatter rather than trusting
		// p.Ticket (populated once at claim time, before the agent ran): the
		// agent may have called `gx tickets set --iteration-status finished
		// --commitless true` on itself during this iteration to declare the
		// zero-commit finish intentional. iteration_status: finished is the
		// "this was deliberate" signal — not a non-claimed status, which an
		// agent can no longer write itself (see ticket 11's CLI guard) — so a
		// commitless ticket that never reported finished falls through to the
		// paused (answer) path below like any other zero-commit finish. This is
		// the one place gx writes status: done with no cherry-pick, so it does
		// so itself here rather than trusting the ticket file to already say
		// done.
		commitlessAdopted, err := adoptCommitlessFinish(d, p, path, pane, sessionID)
		if err != nil {
			return err
		}
		if commitlessAdopted {
			return finishCleanup(d, p.RepoDir, p.FeatureWorktree, path, branch, iterationSession(p, pane), true)
		}

		// A zero-commit finish whose last turn is shaped like ticket 01's
		// unexecuted-tool-call glitch gets one corrective nudge and re-wait
		// before this falls to paused (answer) — see
		// retryUnexecutedToolCallOnce. A non-match, a pane that isn't in a
		// plain finish state any more, or a retry that itself paused on a
		// blocked pane all skip straight past this unchanged.
		retried, newAhead, newSessionID, err := retryUnexecutedToolCallOnce(d, p, path, pane, base, branch, sessionID)
		if err != nil {
			if errors.Is(err, supervise.ErrBlockedPanePaused) {
				return nil
			}
			return err
		}
		if retried {
			ahead = newAhead
			sessionID = newSessionID

			// The retry's own turn is a second, independent turn: it can end
			// with its own iteration_status self-report that has nothing to
			// do with the original glitch (see ADR 0019). Re-run the same
			// adoption checks that already ran pre-retry so that report isn't
			// silently dropped into the ordinary zero-commit/merge handling
			// below.
			adopted, err := adoptAskingReport(p, path, pane, sessionID)
			if err != nil {
				return err
			}
			if adopted && p.KeepAskedAgent {
				return nil
			}
			if adopted {
				return stopAskedSession(d, p, pane)
			}

			if ahead == 0 {
				commitlessAdopted, err := adoptCommitlessFinish(d, p, path, pane, sessionID)
				if err != nil {
					return err
				}
				if commitlessAdopted {
					return finishCleanup(d, p.RepoDir, p.FeatureWorktree, path, branch, iterationSession(p, pane), true)
				}
			}
		}

		if ahead == 0 {
			// The agent finished without merging any commits: leave the worktree/
			// tab in place for inspection instead of silently marking done or
			// retrying, and let the scheduler move on to other unblocked tickets.
			if _, err := p.pauseAsking(events.ZeroCommit, zeroCommitReason(d, path), pane, sessionID, path); err != nil {
				return fmt.Errorf("pausing ticket to ask: %w", err)
			}
			return nil
		}
	}

	// Merging itself (cherry-pick, conflict resolution, mark-done, cleanup) is
	// deferred to the merge-queue worker rather than run inline here: that's
	// what lets this build's active slot free up immediately instead of
	// holding it through up to 30 minutes of conflict resolution (see
	// mergequeue.go). iteration_status: finished is gx's own signal that this
	// build has commits ready to merge (see MarkBuiltAwaitingMerge's doc for how
	// this stays discriminable from an agent's own commitless self-report);
	// Status stays claimed. builtAwaitingMergeError carries everything the
	// worker needs that only exists as live-process state — pane/tab/
	// sessionID/path aren't available once this build goroutine returns.
	if err := ticketops.MarkBuiltAwaitingMerge(p.Ticket.Path); err != nil {
		return fmt.Errorf("marking ticket %s built awaiting merge: %w", p.Ticket.Identifier, err)
	}
	return &builtAwaitingMergeError{job: mergeJob{
		ticket:    p.Ticket,
		base:      base,
		branch:    branch,
		sessionID: sessionID,
		pane:      pane,
		path:      path,
	}}
}

// iterationCommitsAhead counts the iteration's commits on branch. An agent
// that detached HEAD in the worktree (e.g. `git checkout HEAD~0`) commits past
// branch, so branch is first moved forward to HEAD; otherwise that work would
// read as a zero-commit finish and never merge. The move is best effort: if it
// fails, the count is the branch's own, as before.
func iterationCommitsAhead(d deps.Deps, path, base, branch string) (int, error) {
	if d.AttachDetachedHead != nil {
		_, _ = d.AttachDetachedHead(path, branch)
	}
	return d.CommitsAhead(path, base, branch)
}

// zeroCommitReason is the zero-commit pause's reason. It names a dirty worktree,
// since uncommitted work there is what a person most needs to know; a status
// that cannot be read keeps the plain reason.
func zeroCommitReason(d deps.Deps, path string) string {
	const reason = "no commits merged"
	if d.UncommittedChanges == nil {
		return reason
	}
	dirty, err := d.UncommittedChanges(path)
	if err != nil || len(dirty) == 0 {
		return reason
	}
	return fmt.Sprintf("%s; worktree has uncommitted changes (%d files)", reason, len(dirty))
}

// unexecutedToolCallCorrection is the corrective prompt sent to a pane whose
// last turn ended with a text block shaped like an unexecuted tool call: it
// names the failure directly so the agent retries the call instead of
// treating the nudge as a new instruction.
const unexecutedToolCallCorrection = "Your last turn ended with a tool call written as plain text instead of an " +
	"actual tool invocation, so it never ran. Please invoke that tool call for real and continue."

// retryUnexecutedToolCallOnce is finishIteration's single bounded retry for
// ticket 01's unexecuted-tool-call glitch: a zero-commit finish whose
// transcript ends in a text block shaped like an unexecuted tool call is a
// bare model glitch a plain nudge reliably clears, not a genuine stall (see
// ticket 02). It sends one corrective prompt and re-waits for finish, then
// reports the recounted ahead value for finishIteration to resume its
// zero-commit handling with.
//
// retried is false whenever no corrective prompt was sent — the transcript
// doesn't match, or the pane's live status is no longer a plain finish state
// (alreadyFinished) — so finishIteration's caller leaves ahead/sessionID
// untouched and falls straight to paused (answer), same as before this ticket.
// A pane that reads blocked at the moment of the check gets no prompt sent
// at all, honoring the rule that gx never types into a pane sitting on a
// dialog it did not raise; an err wrapping supervise.ErrBlockedPanePaused reports the
// retry's own re-wait pausing after a Codex quota reset.
func retryUnexecutedToolCallOnce(d deps.Deps, p iterationParams, path, pane, base, branch, sessionID string) (retried bool, ahead int, newSessionID string, err error) {
	matched, err := d.ReadUnexecutedToolCall(path, sessionID)
	if err != nil {
		return false, 0, "", fmt.Errorf("reading transcript for unexecuted-tool-call detection: %w", err)
	}
	if !matched {
		return false, 0, "", nil
	}

	label := worktree.IterationLabel(p.FeatureBranch, p.Ticket.Identifier)
	s, found, err := d.Runner.Find(label)
	if err == nil && !found {
		err = agentrunner.ErrNotFound
	}
	var status agentrunner.Status
	if err == nil {
		status, err = d.Runner.Status(s)
	}
	if err != nil {
		return false, 0, "", fmt.Errorf("reading live agent state for %s before corrective retry: %w", label, err)
	}
	if !supervise.AlreadyFinished(string(status.State)) {
		return false, 0, "", nil
	}

	retrySessionID := cmp.Or(status.SessionID, sessionID)
	launchParams := p.launchAndPromptParams(label, pane, "", path, "", "")
	launchParams.VerifyCap = p.verifyCap
	launchParams.Session = s
	if err := supervise.PromptIdle(d, launchParams, unexecutedToolCallCorrection); err != nil {
		return false, 0, "", fmt.Errorf("sending corrective prompt to %s: %w", label, err)
	}

	if err := supervise.WaitForFinish(d, launchParams, retrySessionID); err != nil {
		if errors.Is(err, supervise.ErrBlockedPanePaused) {
			return false, 0, "", err
		}
		return false, 0, "", fmt.Errorf("waiting for %s to finish after corrective retry: %w", label, err)
	}

	newAhead, err := iterationCommitsAhead(d, path, base, branch)
	if err != nil {
		return false, 0, "", fmt.Errorf("counting commits ahead of %s after corrective retry: %w", base, err)
	}
	return true, newAhead, retrySessionID, nil
}

// adoptCommitlessFinish honours a ticket's `gx tickets set --iteration-status
// finished --commitless true` self-report: the "this was deliberate" signal
// for a zero-commit finish (see finishIteration's doc for why iteration_status:
// finished, not just a non-claimed status, is what discriminates this from an
// ordinary stall). It reads the ticket fresh rather than trusting p.Ticket
// (populated once at claim time) because the report is written by the agent
// during the iteration this call is completing. Callers are responsible for
// only invoking this when ahead == 0 — it does not check commit count itself.
func adoptCommitlessFinish(d deps.Deps, p iterationParams, path, pane, sessionID string) (adopted bool, err error) {
	current, err := schema.ParseTicket(p.Ticket.Path)
	if err != nil {
		return false, fmt.Errorf("reading ticket %s for commitless check: %w", p.Ticket.Path, err)
	}
	if !current.IsCommitless() || current.IterationStatus != schema.IterationStatusFinished {
		return false, nil
	}
	stampCommitlessMetrics(d.SessionHomes, p, path, sessionID)
	if err := ticketops.MarkDone(p.Ticket.Path); err != nil {
		return false, fmt.Errorf("marking commitless ticket %s done: %w", p.Ticket.Identifier, err)
	}
	p.logTicketEvent(string(events.Commitless), pane, sessionID, path)
	return true, nil
}

// adoptAskingReport honours an agent's `iteration_status: asking`
// report before finishIteration counts commits, merges them, cleans up, or
// announces the iteration finished (ADR 0019's adoption-precedes-merging
// invariant): without this ordering, an agent that commits what is green and
// then stops to ask has its partial work cherry-picked and marked done while
// the question is still unanswered. Unlike the zero-commit fault path below,
// it never runs zero-commit fault detection and never sets commitless — a
// ticket that stops to ask fully expects to commit after it resumes, so zero
// commits at this point is legal. It reads the ticket fresh rather than
// trusting p.Ticket (populated once at claim time) because the report is
// written by the agent during the iteration this call is completing.
func adoptAskingReport(p iterationParams, path, pane, sessionID string) (adopted bool, err error) {
	current, err := schema.ParseTicket(p.Ticket.Path)
	if err != nil {
		return false, fmt.Errorf("reading ticket %s for iteration-status adoption: %w", p.Ticket.Path, err)
	}
	if current.IterationStatus != schema.IterationStatusAsking {
		return false, nil
	}

	if _, err := p.pauseAsking(events.SelfReported, "agent asked via iteration_status", pane, sessionID, path); err != nil {
		return false, fmt.Errorf("adopting ask report: %w", err)
	}
	return true, nil
}

// pauseAsking routes a finish-time paused (answer) pause through the single
// pause path, carrying the iteration's pane/tab/session on the event.
func (p iterationParams) pauseAsking(kind events.Kind, reason, pane, sessionID, cwd string) (string, error) {
	return ticketops.PauseTicket(p.Sink, ticketops.PauseRequest{
		ScratchDir: p.ScratchDir, EpicName: p.FeatureBranch, Ticket: p.Ticket.Identifier, Path: p.Ticket.Path,
		Type: events.Paused, Kind: kind, Reason: reason,
		Event: p.agentEvent(pane, sessionID, cwd),
	})
}

// markDoneStampingCloseMetadata marks p.Ticket done, stamping the closing
// iteration's compaction count when sessionID is a live, fresh-iteration
// session (a fresh iteration's case) — its peak context window was already
// stamped by mergeCherryPick. A reattached close (sessionID == "") skipped
// that stamp, so it backfills from the ticket's original iteration-started
// session in the run log instead.
func markDoneStampingCloseMetadata(d deps.Deps, p iterationParams, cwd, sessionID string) error {
	if sessionID == "" {
		return backfillDoneMetadata(d, p)
	}
	return markDoneWithCompactions(d, p.Agent, p.Ticket.Path, cwd, sessionID)
}

// backfillDoneMetadata handles a reattached ticket close: it looks up the
// ticket's original iteration-started session from the run log (ticket 06a)
// and stamps the merged metrics and compactions from that session's own
// transcript, rather than leaving the fields blank or attributing them to a
// fresh session that didn't do the work. Falls back to a plain MarkDone
// whenever no prior session can be found — these fields are a best-effort
// convenience, not required for a ticket to close.
func backfillDoneMetadata(d deps.Deps, p iterationParams) error {
	sessionID, sessionCwd, agent, ok := recoverIterationSession(p)
	if !ok {
		return ticketops.MarkDone(p.Ticket.Path)
	}
	if _, _, err := metrics.WriteMergedMetrics(d.SessionHomes, agent, sessionCwd, sessionID, p.Ticket.Path); err != nil {
		return fmt.Errorf("writing merged metrics for ticket %s: %w", p.Ticket.Identifier, err)
	}
	return markDoneWithCompactions(d, agent, p.Ticket.Path, sessionCwd, sessionID)
}

func markDoneWithCompactions(d deps.Deps, agent agent.AgentKind, path, cwd, sessionID string) error {
	compactions, ok, err := supervise.SessionCompactions(d, agent, cwd, sessionID)
	if err != nil || !ok {
		return ticketops.MarkDone(path)
	}
	return ticketops.MarkDoneWithMetadata(path, compactions, sessionID)
}

// recoverIterationSession finds the session of p.Ticket's last
// iteration-started run-log event, for a reattached close that has no live
// session of its own.
func recoverIterationSession(p iterationParams) (sessionID, cwd string, agent agent.AgentKind, ok bool) {
	events, ok, err := runlog.ReadEvents(p.ScratchDir, p.FeatureBranch)
	if err != nil || !ok {
		return "", "", "", false
	}
	return lastIterationSession(events, p.Ticket.Identifier)
}

// lastIterationSession finds the most recent iteration-started event for
// identifier (a ticket's Identifier, not Number, so lettered split siblings
// aren't cross-attributed) with a recorded AgentSession — the session
// id/cwd/agent to backfill a reattached ticket close's metadata with (ticket
// 06a), since the reattaching run never captured a fresh session of its own.
// Agent defaults to agent.AgentClaude for historical logs that omitted it (see
// runlog.Event.Agent).
func lastIterationSession(evs []runlog.Event, identifier string) (agentSession, cwd string, agentKind agent.AgentKind, ok bool) {
	all := runlog.IterationSessions(evs, identifier)
	if len(all) == 0 {
		return "", "", "", false
	}
	return all[0].Session, all[0].Cwd, all[0].Agent, true
}

// stampCommitlessMetrics writes actual_context_window/elapsed_time for a
// commitless finish, the ahead==0 counterpart to mergeCherryPick's
// writeMergedMetrics call on the committed path. A fresh iteration's own
// sessionID is read directly; a reattached close (sessionID == "") recovers
// its session the same way backfillDoneMetadata does. Metrics here are
// best-effort — any failure to find or read a session leaves the fields at
// their existing zero value rather than failing the finish, since the ticket
// is already done and these fields are a display convenience, not a
// precondition for closing it.
func stampCommitlessMetrics(homes metrics.SessionHomes, p iterationParams, cwd, sessionID string) {
	if sessionID != "" {
		metrics.WriteMergedMetrics(homes, p.Agent, cwd, sessionID, p.Ticket.Path)
		return
	}
	sid, sessionCwd, agent, ok := recoverIterationSession(p)
	if !ok {
		// No session to recover: still stamp the ticket's own session_ids
		// (zero when it has none).
		metrics.WriteMergedMetrics(homes, p.Agent, "", "", p.Ticket.Path)
		return
	}
	metrics.WriteMergedMetrics(homes, agent, sessionCwd, sid, p.Ticket.Path)
}

// mergeCherryPick cherry-picks base..branch onto the feature branch (resolving
// conflicts via cherryPickWithConflictResolution if any arise), then resolves
// the SHA it merged at — the shared core of both a normal iteration's
// completion (the merge-queue worker) and a startup repair's re-cherry-pick
// (repairRecoverableTicket). Callers serialize merges themselves (the
// merge-queue worker is a single goroutine; reconcile runs single-threaded at
// startup before it starts), so this function needs no lock of its own.
// Before resolving the SHA, it writes the merging iteration's session
// metrics (actual_context_window, elapsed_time) into the ticket's
// frontmatter via writeMergedMetrics, then stamps ticketTrailerKey and —
// when those metrics were available — the same values onto
// tokensTrailerKey/elapsedTrailerKey, all in a single amend
// (Deps.AppendTrailers) rather than one amend per trailer.
func mergeCherryPick(d deps.Deps, p iterationParams, base, branch, sessionID, pane string) (string, error) {
	res, resolutionSessionID, err := cherryPickWithConflictResolution(d, p, base, branch, sessionID, pane)
	if err != nil {
		return "", err
	}
	if resolutionSessionID != "" {
		p.logTicketEventSHA(string(events.ConflictResolved), "", resolutionSessionID, p.FeatureWorktree, "", res.SHA)
	}
	return res.SHA, nil
}

// stopAskedSession ends the agent session of an iteration that asked on a
// non-keeping run. The worktree and branch stay: the relaunch after the
// answer reuses them as they are, uncommitted files included.
func stopAskedSession(d deps.Deps, p iterationParams, pane string) error {
	s := iterationSession(p, pane)
	if s == (agentrunner.Session{}) {
		return nil
	}
	if err := d.Runner.Stop(s); err != nil {
		return fmt.Errorf("stopping iteration session: %w", err)
	}
	return nil
}

// finishCleanup removes an iteration's now-redundant worktree and tab, and —
// when removeBranch is true — its branch too. removeBranch is false for a
// paused (paused (answer)) iteration: the branch is what a later resume
// reattaches to, so it must outlive the worktree/tab/permit that pause drops
// (see finishIteration's adopted-report path). Each check is independent
// since callers run this against state left in different shapes: a normal
// completion (worktree/tab/branch all just created and definitely present),
// or a done ticket's leftover state found on startup after a crash (any
// subset may have survived — see classifyDoneTicket's doneStaleCleanup).
// s is the zero Session if no live session was found for this iteration.
func finishCleanup(d deps.Deps, repoDir, featureWorktree, path, branch string, s agentrunner.Session, removeBranch bool) error {
	hasWorktree, err := d.WorktreeExists(path)
	if err != nil {
		return fmt.Errorf("checking iteration worktree: %w", err)
	}
	if hasWorktree {
		worktree.Lock.Lock()
		err := d.RemoveWorktree(repoDir, path, true)
		worktree.Lock.Unlock()
		if err != nil {
			return fmt.Errorf("removing iteration worktree: %w", err)
		}
	}

	if s != (agentrunner.Session{}) {
		if err := d.Runner.Stop(s); err != nil {
			return fmt.Errorf("stopping iteration session: %w", err)
		}
	}

	if removeBranch && worktree.BranchExists(d, featureWorktree, branch) {
		if err := d.DeleteBranch(repoDir, branch); err != nil {
			return fmt.Errorf("deleting iteration branch %s: %w", branch, err)
		}
	}

	return nil
}
