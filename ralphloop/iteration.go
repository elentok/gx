package ralphloop

import (
	"cmp"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// conflictResolutionTimeoutMs bounds how long a conflict-resolution agent may
// run before it's treated as stuck, so a hung resolution surfaces as a
// distinct, actionable error instead of hanging the whole loop forever.
const conflictResolutionTimeoutMs = 30 * 60 * 1000

// runIteration drives one ticket through the full iteration lifecycle:
// create its worktree, launch and prompt the agent, wait for it to finish,
// adopt any iteration_status report the agent left, cherry-pick its commits
// onto the feature branch, mark the ticket done, and remove the iteration
// worktree. An `iteration_status: needs-answer` report is adopted before any
// of that: see adoptNeedsAnswerReport. Otherwise, if the agent finishes
// without landing any commits, the ticket is marked needs-answer instead and
// the worktree/tab are left in place for inspection — unless the agent
// itself declared the zero-commit finish intentional via `gx tickets set
// --iteration-status finished --commitless true`, in which case gx itself
// writes status: done and the worktree/tab are cleaned up normally with no
// commit landed.
func runIteration(d Deps, p iterationParams) error {
	label := iterLabel(p.FeatureBranch, p.Ticket.Identifier)
	branch := iterBranch(p.FeatureBranch, p.Ticket.Identifier)
	path := iterationWorktreePath(p.WorktreeDir, p.FeatureBranch, p.Ticket.Identifier)

	// A branch surviving under this name means a prior park dropped this
	// ticket's worktree/tab but kept its branch, per finishIteration's
	// needs-answer adoption cleanup. Landing that resumed iteration's commits
	// — both the ones from before the park and whatever it adds after —
	// needs the merge base of the two branches, not the feature tip: basing
	// off today's tip would silently drop the pre-park commits, since the
	// feature branch has very likely advanced past the point this branch
	// forked from.
	var base string
	var err error
	if branchExists(d, p.FeatureWorktree, branch) {
		base, err = d.MergeBase(p.FeatureWorktree, branch, p.FeatureBranch)
		if err != nil {
			return fmt.Errorf("resolving %s's original base: %w", branch, err)
		}
	} else {
		base, err = d.RevParse(p.FeatureWorktree, p.FeatureBranch)
		if err != nil {
			return fmt.Errorf("resolving %s tip: %w", p.FeatureBranch, err)
		}
	}

	p.WorktreeLock.Lock()
	err = d.AddWorktree(p.RepoDir, path, branch, base)
	p.WorktreeLock.Unlock()
	if err != nil {
		return fmt.Errorf("creating iteration worktree: %w", err)
	}

	// Runs synchronously here, before the agent's tab/session exist, so an
	// install failure surfaces as an iteration-lifecycle error immediately
	// rather than eating into the agent's own turn or smart-zone budget.
	command, err := d.InstallDeps(path)
	if err != nil {
		return fmt.Errorf("installing dependencies in %s: %w", path, err)
	}
	p.logTicketEventReason(string(events.DepsInstalled), "", "", "", path, command)

	skill := p.Skill
	if p.Ticket.IsCodeReview() {
		skill = codeReviewSkill
	}
	prompt := skillPrompt(p.Agent, skill, ticketAddress(p.Ticket))

	l, err := startAndPrompt(d.Runner, agentrunner.StartOptions{
		Label: label,
		Epic:  p.FeatureBranch,
		Cwd:   path,
		Kind:  agentrunner.Kind(p.Agent),
		Args:  agentArgs(p.Agent, p.ScratchDir, p.FeatureBranch, p.Model, p.Effort),
	}, prompt, func(attempt int, kind events.Kind, err error) {
		p.logLaunchFailed(label, attempt, kind, err)
	})
	var launchFail *launchFailure
	if errors.As(err, &launchFail) {
		return err
	}
	launchParams := p.launchAndPromptParams(label, l.ID, iterationTabID(d, label), prompt, path, string(events.IterationStarted), string(events.IterationFinished))
	launchParams.Session = l.Session
	var sessionID string
	if errors.Is(err, agentrunner.ErrNotReady) {
		// The agent is live but blocked on a dialog gx did not raise: same
		// park as a mid-turn block. One that cleared before the re-check is a
		// plain launch failure.
		parked, parkErr := parkOnBlockedPane(d, launchParams, l.SessionID)
		if parkErr != nil {
			return parkErr
		}
		if parked {
			sessionID, err = l.SessionID, errBlockedPaneParked
		}
	}
	switch {
	case errors.Is(err, errBlockedPaneParked):
	case err != nil:
		err = fmt.Errorf("sending initial prompt: %w", err)
		p.logLaunchFailed(label, 1, events.IterationError, err)
		return &launchFailure{Kind: events.IterationError, Err: err}
	case l.Adopted:
		sessionID, err = adoptedLaunch(d, launchParams)
	default:
		sessionID, err = promptedLaunch(d, launchParams, l.Baseline)
	}
	parked := errors.Is(err, errBlockedPaneParked)
	if err != nil && !parked {
		return err
	}
	if sessionID != "" {
		if err := AppendSessionID(p.Ticket.Path, sessionID); err != nil {
			return fmt.Errorf("appending session id for ticket %s: %w", p.Ticket.Identifier, err)
		}
	}
	if parked {
		// The park already ended the iteration: the agent is still live in its
		// pane, so finishIteration's commit-check/cherry-pick/cleanup — which
		// assumes the agent actually finished — must not run. Only gx's
		// watcher goes away; the pane, tab, and worktree survive for a person
		// to answer in the pane.
		return nil
	}

	return finishIteration(d, p, path, launchParams.Pane, launchParams.Tab, base, branch, sessionID)
}

// iterationSession is the live session of p's ticket, for cleanup; the zero
// Session when the iteration has no pane.
func iterationSession(p iterationParams, pane string) agentrunner.Session {
	if pane == "" {
		return agentrunner.Session{}
	}
	return agentrunner.Session{Label: iterLabel(p.FeatureBranch, p.Ticket.Identifier), ID: pane}
}

// iterationTabID is the tab hosting label's agent, for finishIteration's
// cleanup. Runner sessions don't expose their tab, so herdr is asked by name;
// on failure the tab is left open rather than failing a launched iteration.
func iterationTabID(d Deps, label string) string {
	agent, err := d.AgentGet(label)
	if err != nil {
		log.Printf("resolving %s's tab: %v", label, err)
		return ""
	}
	return agent.TabID
}

// reattachIteration resumes a claimed ticket whose worktree, tab, and agent
// survived a prior invocation. The stable iteration label locates the live
// agent, while its recovered pane and native session identity drive the
// remaining wait, lifecycle logging, and completion work. Codex sessions are
// accepted only when their rollout metadata belongs to this worktree.
func reattachIteration(d Deps, p iterationParams) error {
	label := iterLabel(p.FeatureBranch, p.Ticket.Identifier)
	branch := iterBranch(p.FeatureBranch, p.Ticket.Identifier)
	path := iterationWorktreePath(p.WorktreeDir, p.FeatureBranch, p.Ticket.Identifier)

	// A reattach that stays claimed throughout (the common case) never goes
	// through Claim, so a stale iteration_status left by a pre-restart report
	// must be cleared here instead - unconditionally, so it can't survive
	// into the new attach before finishIteration/waitForFinish run. The
	// pre-clear value is kept: if the agent turns out to be alreadyFinished
	// below, there is no further run of it to produce a fresh report, so the
	// cleared value is this iteration's only report and is restored before
	// finishIteration reads it - otherwise a genuine finished+commitless
	// report left by an agent that exited while the loop itself was down
	// would be wiped by this same clear and wrongly fall to needs-answer.
	preClear, err := schema.ParseTicket(p.Ticket.Path)
	if err != nil {
		return fmt.Errorf("reading ticket %s before clearing iteration_status: %w", p.Ticket.Identifier, err)
	}
	priorIterationStatus := preClear.IterationStatus
	if err := schema.ClearIterationStatus(p.Ticket.Path); err != nil {
		return fmt.Errorf("clearing iteration_status for reattached ticket %s: %w", p.Ticket.Identifier, err)
	}

	session, found, err := d.Runner.Find(label)
	if err != nil {
		return fmt.Errorf("finding live session for reattached iteration %s: %w", label, err)
	}
	if !found {
		return fmt.Errorf("no live session found for reattached iteration %s", label)
	}
	agent, err := d.Runner.Status(session)
	if err != nil {
		return fmt.Errorf("reattached iteration %s: %w", label, err)
	}
	tabID := iterationTabID(d, label)
	if p.Agent == AgentCodex {
		if agent.SessionID == "" {
			return fmt.Errorf("missing live Codex session for reattached iteration %s; keep the tab open and retry after Herdr reports its session", label)
		}
		verified, verifyErr := d.VerifyCodexSession(path, agent.SessionID)
		if verifyErr != nil {
			return fmt.Errorf("verifying live Codex session %s for reattached iteration %s: %w", agent.SessionID, label, verifyErr)
		}
		if !verified {
			return fmt.Errorf("live Codex session %s for reattached iteration %s does not match rollout metadata for cwd %s", agent.SessionID, label, path)
		}
	}
	p.Sink.TicketReattached(p.Ticket.Identifier, label, path, agent.SessionID)
	if agent.SessionID != "" {
		if err := AppendSessionID(p.Ticket.Path, agent.SessionID); err != nil {
			return fmt.Errorf("appending session id for reattached ticket %s: %w", p.Ticket.Identifier, err)
		}
	}

	base, err := d.MergeBase(path, branch, p.FeatureBranch)
	if err != nil {
		return fmt.Errorf("resolving %s's original base: %w", branch, err)
	}

	// StartEvent remains empty because reattachment must not imply a fresh
	// launch; all later events use the recovered native session identity.
	launchParams := p.launchAndPromptParams(label, session.ID, tabID, "", path, "", string(events.IterationFinished))
	finished := false
	if alreadyFinished(string(agent.State)) {
		// An idle pane at reattach gets the same debounce and background-task
		// gate a live finish gets in waitForFinish (confirmFinished /
		// waitForBackgroundTasks): without them, a genuine mid-turn pause or a
		// still-outstanding backgrounded shell command reads as finished,
		// landing a partial commit set and tearing down the worktree under a
		// still-working agent - which is how both real park/reopen/reattach
		// incidents actually looped, since this short-circuit used to skip
		// waitForFinish (and its gate) entirely.
		confirmed, err := confirmFinished(d, launchParams.session(), runnerFinishStates)
		if err != nil {
			return fmt.Errorf("confirming %s already finished at reattach: %w", label, err)
		}
		if confirmed {
			sessionID := resolveReattachSessionID(p, agent.SessionID, preClear.SessionIDs)
			elapsedMs := 0
			finishedAfterGate, err := waitForBackgroundTasks(d, launchParams, sessionID, runnerFinishStates, &elapsedMs)
			if err != nil {
				return err
			}
			finished = finishedAfterGate
		}
	}
	if finished {
		// An agent that finished while the loop was down has no future state
		// transition for AgentWait to observe.
		if p.Report != nil {
			p.Report("%s already finished at reattach; skipping wait\n", label)
		}
		if priorIterationStatus != "" {
			if err := schema.UpdateTicket(p.Ticket.Path, func(t *schema.Ticket) {
				t.IterationStatus = priorIterationStatus
			}); err != nil {
				return fmt.Errorf("restoring iteration_status for already-finished reattached ticket %s: %w", p.Ticket.Identifier, err)
			}
		}
		launchParams.logLifecycleEvent(launchParams.FinishEvent, agent.SessionID)
	} else if err := waitForFinish(d, launchParams, agent.SessionID); err != nil {
		if errors.Is(err, errBlockedPaneParked) {
			// Same as runIteration's park: the iteration ends here, the pane/
			// tab/worktree survive, and finishIteration must not run.
			return nil
		}
		return fmt.Errorf("waiting for reattached agent %s to finish: %w", label, err)
	}
	if strings.EqualFold(strings.TrimSpace(p.Ticket.Status), "needs-repair") {
		if err := Claim(p.Ticket.Path); err != nil {
			return fmt.Errorf("restoring ticket to claimed: %w", err)
		}
		p.Sink.IterationResumed(p.Ticket.Identifier, label, PauseNeedsRepair)
		if p.Report != nil {
			p.Report("resumed %s after restart recheck\n", label)
		}
		launchParams.logLifecycleEvent(string(events.Resumed), agent.SessionID)
		p.Gate.ForceResume(label)
	}

	return finishIteration(d, p, path, session.ID, tabID, base, branch, agent.SessionID)
}

// resolveReattachSessionID recovers a session id for
// reattachIteration's already-finished background-task gate check: Claude's
// AgentSession is empty at reattach until the live agent produces one, so
// this falls back through the same chain established for a reattached
// close's metadata (backfillDoneMetadata) - the ticket's own most recently
// recorded session (priorSessionIDs, read before this reattach could have
// appended one of its own), then the run log's last iteration-started event.
// Returns "" if none of those resolve, which waitForBackgroundTasks treats
// as no session to gate on rather than an error.
func resolveReattachSessionID(p iterationParams, agentSession string, priorSessionIDs []string) string {
	if agentSession != "" {
		return agentSession
	}
	if n := len(priorSessionIDs); n > 0 {
		return priorSessionIDs[n-1]
	}
	events, ok, err := ReadEvents(p.ScratchDir, p.FeatureBranch)
	if err != nil || !ok {
		return ""
	}
	sessionID, _, _, ok := lastIterationSession(events, p.Ticket.Identifier)
	if !ok {
		return ""
	}
	return sessionID
}

// finishIteration lands a finished iteration's commits (or marks it
// needs-answer if it produced none), then removes its worktree/tab on success.
// Fresh and reattached iterations both carry their native session and pane
// identity through this shared completion path. A needs-answer report is
// adopted before any of that: see adoptNeedsAnswerReport.
func finishIteration(d Deps, p iterationParams, path, pane, tab, base, branch, sessionID string) error {
	adopted, err := adoptNeedsAnswerReport(p, path, pane, tab, sessionID)
	if err != nil {
		return err
	}
	if adopted {
		// The branch is what makes a later resume able to land both sides of
		// the answer boundary in one pick (see runIteration's branch-reuse
		// base computation); only the worktree/tab/permit are redundant while
		// the ticket waits on an answer.
		return finishCleanup(d, p.WorktreeLock, p.RepoDir, p.FeatureWorktree, path, branch, iterationSession(p, pane), false)
	}

	ahead, err := d.CommitsAhead(path, base, branch)
	if err != nil || ahead == 0 {
		// waitForFinish's own debounce (confirmFinished) already guards against
		// herdr reporting the agent idle mid-turn, but a commit can still land
		// in the gap between that confirmation and this check (e.g. a reattached
		// iteration, which skips waitForFinish's launch-time debounce entirely).
		// The count can also fail transiently right after the pane exits (e.g.
		// a worktree/ref momentarily unresolvable during concurrent reconcile
		// activity) even though the agent's work already landed. Recheck once
		// more before giving up rather than orphaning a commit — or stranding
		// an otherwise-finished ticket — on a one-off blip.
		d.Sleep(finishDebounceMs * time.Millisecond)
		ahead, err = d.CommitsAhead(path, base, branch)
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
		// needs-answer path below like any other zero-commit finish. This is
		// the one place gx writes status: done with no cherry-pick, so it does
		// so itself here rather than trusting the ticket file to already say
		// done.
		commitlessAdopted, err := adoptCommitlessFinish(p, path, pane, tab, sessionID)
		if err != nil {
			return err
		}
		if commitlessAdopted {
			return finishCleanup(d, p.WorktreeLock, p.RepoDir, p.FeatureWorktree, path, branch, iterationSession(p, pane), true)
		}

		// A zero-commit finish whose last turn is shaped like ticket 01's
		// unexecuted-tool-call glitch gets one corrective nudge and re-wait
		// before this falls to needs-answer — see
		// retryUnexecutedToolCallOnce. A non-match, a pane that isn't in a
		// plain finish state any more, or a retry that itself parked on a
		// blocked pane all skip straight past this unchanged.
		retried, newAhead, newSessionID, err := retryUnexecutedToolCallOnce(d, p, path, pane, tab, base, branch, sessionID)
		if err != nil {
			if errors.Is(err, errBlockedPaneParked) {
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
			// silently dropped into the ordinary zero-commit/land handling
			// below.
			adopted, err := adoptNeedsAnswerReport(p, path, pane, tab, sessionID)
			if err != nil {
				return err
			}
			if adopted {
				return finishCleanup(d, p.WorktreeLock, p.RepoDir, p.FeatureWorktree, path, branch, iterationSession(p, pane), false)
			}

			if ahead == 0 {
				commitlessAdopted, err := adoptCommitlessFinish(p, path, pane, tab, sessionID)
				if err != nil {
					return err
				}
				if commitlessAdopted {
					return finishCleanup(d, p.WorktreeLock, p.RepoDir, p.FeatureWorktree, path, branch, iterationSession(p, pane), true)
				}
			}
		}

		if ahead == 0 {
			// The agent finished without landing any commits: leave the worktree/
			// tab in place for inspection instead of silently marking done or
			// retrying, and let the scheduler move on to other unblocked tickets.
			if _, err := p.parkNeedsAnswer(events.ZeroCommit, "no commits landed", pane, tab, sessionID, path); err != nil {
				return fmt.Errorf("marking ticket needs-answer: %w", err)
			}
			return nil
		}
	}

	// Landing itself (cherry-pick, conflict resolution, mark-done, cleanup) is
	// deferred to the land-queue worker rather than run inline here: that's
	// what lets this build's active slot free up immediately instead of
	// holding it through up to 30 minutes of conflict resolution (see
	// landqueue.go). iteration_status: finished is gx's own signal that this
	// build has commits ready to land (see MarkBuiltAwaitingLand's doc for how
	// this stays discriminable from an agent's own commitless self-report);
	// Status stays claimed. builtAwaitingLandError carries everything the
	// worker needs that only exists as live-process state — pane/tab/
	// sessionID/path aren't available once this build goroutine returns.
	if err := MarkBuiltAwaitingLand(p.Ticket.Path); err != nil {
		return fmt.Errorf("marking ticket %s built awaiting land: %w", p.Ticket.Identifier, err)
	}
	return &builtAwaitingLandError{job: landJob{
		ticket:    p.Ticket,
		base:      base,
		branch:    branch,
		sessionID: sessionID,
		pane:      pane,
		tab:       tab,
		path:      path,
	}}
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
// untouched and falls straight to needs-answer, same as before this ticket.
// A pane that reads blocked at the moment of the check gets no prompt sent
// at all, honoring the rule that gx never types into a pane sitting on a
// dialog it did not raise (see parkOnBlockedPane); an err wrapping
// errBlockedPaneParked reports the retry's own re-wait parking the same way.
func retryUnexecutedToolCallOnce(d Deps, p iterationParams, path, pane, tab, base, branch, sessionID string) (retried bool, ahead int, newSessionID string, err error) {
	matched, err := d.ReadUnexecutedToolCall(path, sessionID)
	if err != nil {
		return false, 0, "", fmt.Errorf("reading transcript for unexecuted-tool-call detection: %w", err)
	}
	if !matched {
		return false, 0, "", nil
	}

	label := iterLabel(p.FeatureBranch, p.Ticket.Identifier)
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
	if !alreadyFinished(string(status.State)) {
		return false, 0, "", nil
	}

	if err := d.Runner.Prompt(s, unexecutedToolCallCorrection); err != nil {
		return false, 0, "", fmt.Errorf("sending corrective prompt to %s: %w", label, err)
	}
	retrySessionID := cmp.Or(status.SessionID, sessionID)
	launchParams := p.launchAndPromptParams(label, pane, tab, "", path, "", "")

	if err := waitForFinish(d, launchParams, retrySessionID); err != nil {
		if errors.Is(err, errBlockedPaneParked) {
			return false, 0, "", err
		}
		return false, 0, "", fmt.Errorf("waiting for %s to finish after corrective retry: %w", label, err)
	}

	newAhead, err := d.CommitsAhead(path, base, branch)
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
func adoptCommitlessFinish(p iterationParams, path, pane, tab, sessionID string) (adopted bool, err error) {
	current, err := schema.ParseTicket(p.Ticket.Path)
	if err != nil {
		return false, fmt.Errorf("reading ticket %s for commitless check: %w", p.Ticket.Path, err)
	}
	if !current.IsCommitless() || current.IterationStatus != schema.IterationStatusFinished {
		return false, nil
	}
	stampCommitlessMetrics(p, path, sessionID)
	if err := MarkDone(p.Ticket.Path); err != nil {
		return false, fmt.Errorf("marking commitless ticket %s done: %w", p.Ticket.Identifier, err)
	}
	p.logTicketEvent(string(events.Commitless), pane, tab, sessionID, path)
	return true, nil
}

// adoptNeedsAnswerReport honours an agent's `iteration_status: needs-answer`
// report before finishIteration counts commits, lands them, cleans up, or
// announces the iteration finished (ADR 0019's adoption-precedes-landing
// invariant): without this ordering, an agent that commits what is green and
// then stops to ask has its partial work cherry-picked and marked done while
// the question is still unanswered. Unlike the zero-commit fault path below,
// it never runs zero-commit fault detection and never sets commitless — a
// ticket that stops to ask fully expects to commit after it resumes, so zero
// commits at this point is legal. It reads the ticket fresh rather than
// trusting p.Ticket (populated once at claim time) because the report is
// written by the agent during the iteration this call is completing.
func adoptNeedsAnswerReport(p iterationParams, path, pane, tab, sessionID string) (adopted bool, err error) {
	current, err := schema.ParseTicket(p.Ticket.Path)
	if err != nil {
		return false, fmt.Errorf("reading ticket %s for iteration-status adoption: %w", p.Ticket.Path, err)
	}
	if current.IterationStatus != schema.IterationStatusNeedsAnswer {
		return false, nil
	}

	if _, err := p.parkNeedsAnswer(events.SelfReported, "agent reported needs-answer via iteration_status", pane, tab, sessionID, path); err != nil {
		return false, fmt.Errorf("adopting needs-answer report: %w", err)
	}
	return true, nil
}

// parkNeedsAnswer routes a finish-time needs-answer park through the single
// park path, carrying the iteration's pane/tab/session on the event.
func (p iterationParams) parkNeedsAnswer(kind events.Kind, reason, pane, tab, sessionID, cwd string) (string, error) {
	return park(p.Sink, parkRequest{
		ScratchDir: p.ScratchDir, EpicName: p.FeatureBranch, Ticket: p.Ticket.Identifier, Path: p.Ticket.Path,
		Type: events.NeedsAnswer, Kind: kind, Reason: reason,
		Event: Event{Agent: p.Agent, Pane: pane, Tab: tab, AgentSession: sessionID, Cwd: cwd},
	})
}

// markDoneStampingCloseMetadata marks p.Ticket done, stamping the closing
// iteration's context-window occupancy and session id alongside Status when
// sessionID is a live, fresh-iteration session (runIteration's case) and its
// occupancy is available — a reattached close (sessionID == "") has no live
// session of its own to read occupancy from, so it backfills from the
// ticket's original iteration-started session in the run log instead.
func markDoneStampingCloseMetadata(d Deps, p iterationParams, cwd, sessionID string) error {
	if sessionID == "" {
		return backfillDoneMetadata(d, p)
	}
	occupancy, ok, occErr := contextOccupancy(d, p.Agent, cwd, sessionID)
	if occErr != nil || !ok {
		return MarkDone(p.Ticket.Path)
	}
	compactions, _, _ := sessionCompactions(d, p.Agent, cwd, sessionID)
	return MarkDoneWithMetadata(p.Ticket.Path, occupancy, compactions, sessionID)
}

// backfillDoneMetadata handles a reattached ticket close: it looks up the
// ticket's original iteration-started session from the run log (ticket 06a)
// and stamps context-window/session from that session's own transcript,
// rather than leaving the fields blank or attributing them to a fresh
// session that didn't do the work. Falls back to a plain MarkDone whenever
// no prior session can be found, or its occupancy can't be read — these
// fields are a best-effort convenience, not required for a ticket to close.
func backfillDoneMetadata(d Deps, p iterationParams) error {
	events, ok, err := ReadEvents(p.ScratchDir, p.FeatureBranch)
	if err != nil || !ok {
		return MarkDone(p.Ticket.Path)
	}
	sessionID, sessionCwd, agent, ok := lastIterationSession(events, p.Ticket.Identifier)
	if !ok {
		return MarkDone(p.Ticket.Path)
	}
	occupancy, ok, occErr := contextOccupancy(d, agent, sessionCwd, sessionID)
	if occErr != nil || !ok {
		return MarkDone(p.Ticket.Path)
	}
	compactions, _, _ := sessionCompactions(d, agent, sessionCwd, sessionID)
	return MarkDoneWithMetadata(p.Ticket.Path, occupancy, compactions, sessionID)
}

// stampCommitlessMetrics writes actual_context_window/elapsed_time for a
// commitless finish, the ahead==0 counterpart to landCherryPick's
// writeLandedMetrics call on the committed path. A fresh iteration's own
// sessionID is read directly; a reattached close (sessionID == "") recovers
// its session the same way backfillDoneMetadata does. Metrics here are
// best-effort — any failure to find or read a session leaves the fields at
// their existing zero value rather than failing the finish, since the ticket
// is already done and these fields are a display convenience, not a
// precondition for closing it.
func stampCommitlessMetrics(p iterationParams, cwd, sessionID string) {
	if sessionID != "" {
		writeLandedMetrics(p.Agent, cwd, sessionID, p.Ticket.Path)
		return
	}
	events, ok, err := ReadEvents(p.ScratchDir, p.FeatureBranch)
	if err != nil || !ok {
		return
	}
	sid, sessionCwd, agent, ok := lastIterationSession(events, p.Ticket.Identifier)
	if !ok {
		return
	}
	writeLandedMetrics(agent, sessionCwd, sid, p.Ticket.Path)
}

// landCherryPick cherry-picks base..branch onto the feature branch (resolving
// conflicts via cherryPickWithConflictResolution if any arise), then resolves
// the SHA it landed at — the shared core of both a normal iteration's
// completion (the land-queue worker) and a startup repair's re-cherry-pick
// (repairRecoverableTicket). Callers serialize landings themselves (the
// land-queue worker is a single goroutine; reconcile runs single-threaded at
// startup before it starts), so this function needs no lock of its own.
// Before resolving the SHA, it writes the landing iteration's session
// metrics (actual_context_window, elapsed_time) into the ticket's
// frontmatter via writeLandedMetrics, then stamps ticketTrailerKey and —
// when those metrics were available — the same values onto
// tokensTrailerKey/elapsedTrailerKey, all in a single amend
// (Deps.AppendTrailers) rather than one amend per trailer.
func landCherryPick(d Deps, p iterationParams, base, branch, sessionID, pane, tab string) (string, error) {
	res, resolutionSessionID, err := cherryPickWithConflictResolution(d, p, base, branch, sessionID, pane, tab)
	if err != nil {
		return "", err
	}
	if resolutionSessionID != "" {
		p.logTicketEventSHA(string(events.ConflictResolved), "", "", resolutionSessionID, p.FeatureWorktree, "", res.SHA)
	}
	return res.SHA, nil
}

// finishCleanup removes an iteration's now-redundant worktree and tab, and —
// when removeBranch is true — its branch too. removeBranch is false for a
// parked (needs-answer) iteration: the branch is what a later resume
// reattaches to, so it must outlive the worktree/tab/permit that park drops
// (see finishIteration's adopted-report path). Each check is independent
// since callers run this against state left in different shapes: a normal
// completion (worktree/tab/branch all just created and definitely present),
// or a done ticket's leftover state found on startup after a crash (any
// subset may have survived — see classifyDoneTicket's doneStaleCleanup).
// s is the zero Session if no live session was found for this iteration.
func finishCleanup(d Deps, worktreeLock *sync.Mutex, repoDir, featureWorktree, path, branch string, s agentrunner.Session, removeBranch bool) error {
	hasWorktree, err := d.WorktreeExists(path)
	if err != nil {
		return fmt.Errorf("checking iteration worktree: %w", err)
	}
	if hasWorktree {
		worktreeLock.Lock()
		err := d.RemoveWorktree(repoDir, path, true)
		worktreeLock.Unlock()
		if err != nil {
			return fmt.Errorf("removing iteration worktree: %w", err)
		}
	}

	if s != (agentrunner.Session{}) {
		if err := d.Runner.Stop(s); err != nil {
			return fmt.Errorf("stopping iteration session: %w", err)
		}
	}

	if removeBranch && branchExists(d, featureWorktree, branch) {
		if err := d.DeleteBranch(repoDir, branch); err != nil {
			return fmt.Errorf("deleting iteration branch %s: %w", branch, err)
		}
	}

	return nil
}

// branchExists reports whether branch is a resolvable ref in dir's repo,
// treating any RevParse error as "doesn't exist" — the same convention
// classifyDoneTicket uses to check the iteration branch's presence.
func branchExists(d Deps, dir, branch string) bool {
	_, err := d.RevParse(dir, branch)
	return err == nil
}

// cherryPickWithConflictResolution cherry-picks base..branch onto
// p.FeatureWorktree. On a conflict, it launches a fresh pane in the feature
// worktree (where the conflict markers are, not the iteration worktree),
// sends "/gx-resolving-merge-conflicts", and waits for that agent to finish
// before confirming the cherry-pick sequence completed. sessionID/pane/tab
// attach the iteration agent's own session to the conflict-hit event (the
// resolution agent hasn't launched yet at that point). On resolution it
// returns that agent's distinct session so landCherryPick can emit
// conflict-resolved with the final post-trailer SHA. The returned result is
// Landed (stamped) or AlreadyApplied, never Conflicted.
func cherryPickWithConflictResolution(d Deps, p iterationParams, base, branch, sessionID, pane, tab string) (res LandResult, resolutionSessionID string, resultErr error) {
	ld, lp := landDepsFor(d), landParamsFor(p, base, branch, sessionID)
	p.Sink.CherryPickStarted(p.Ticket.Identifier)

	// A previous invocation may have died after starting a cherry-pick but
	// before recording its outcome. Never let the next ticket inherit or
	// resolve that operation as its own: its iteration branch is durable, so
	// aborting here is lossless and lets reconciliation retry it explicitly.
	//
	// The one exception is a conflict-resolution agent that survived the
	// crash: its pane is still live in a tab labeled conflictLabel(p.Ticket.
	// Identifier) and still owns this exact sequencer state. Aborting under
	// it would discard its staged edits, and re-cherry-picking would hit the
	// same conflict again and fork a second child ticket onto a TabCreate
	// call sharing that same label — two live tabs on one label, the
	// corruption this guard exists to prevent. Reattach to it instead.
	inProgress, err := d.CherryPickInProgress(p.FeatureWorktree)
	if err != nil {
		return LandResult{}, "", fmt.Errorf("checking for stale cherry-pick onto %s: %w", p.FeatureBranch, err)
	}
	if inProgress {
		liveSession, found, err := findLiveConflictResolutionTab(d, p)
		if err != nil {
			return LandResult{}, "", err
		}
		if found {
			resolutionSessionID, err := reattachLiveConflictResolver(d, p, liveSession)
			if err != nil {
				return LandResult{}, "", err
			}
			res, err := stampLanded(ld, lp)
			return res, resolutionSessionID, err
		}
		// A land marker means the sequencer state is a human's pending
		// conflict, not crash debris: aborting would discard their work.
		lockDir, err := landLockDir(p.ScratchDir, p.FeatureBranch)
		if err != nil {
			return LandResult{}, "", err
		}
		marker, err := ReadLandMarker(lockDir)
		if err != nil {
			return LandResult{}, "", fmt.Errorf("reading land marker: %w", err)
		}
		if marker != nil {
			return LandResult{}, "", errLandDeferred
		}
		// Only gx's own debris is lossless to abort: a pick of some iteration
		// branch's commit. Anything else is a person's work in progress.
		if foreign, sha := foreignCherryPick(d, p); foreign {
			return LandResult{}, "", fmt.Errorf("the feature worktree %s has a cherry-pick of %s in progress that gx did not start; finish or abort it, then land again", p.FeatureWorktree, sha)
		}
		if err := d.AbortCherryPick(p.FeatureWorktree); err != nil {
			return LandResult{}, "", fmt.Errorf("aborting stale cherry-pick onto %s: %w", p.FeatureBranch, err)
		}
	}

	// LandTicket recognizes an iteration whose patch concurrent work already
	// made redundant (git would stop that as an empty cherry-pick and leave
	// sequencer state behind) before touching the worktree.
	res, err = LandTicket(ld, lp)
	if err != nil {
		return LandResult{}, "", err
	}
	if res.Outcome != Conflicted {
		return res, "", nil
	}
	// This call now owns the active sequencer state. Always clean it up on
	// failure, otherwise the next landing serialized behind this one could
	// mistake this ticket's CHERRY_PICK_HEAD for its own conflict.
	defer func() {
		if resultErr == nil {
			return
		}
		active, checkErr := d.CherryPickInProgress(p.FeatureWorktree)
		if checkErr != nil {
			resultErr = fmt.Errorf("%w (also failed checking cherry-pick cleanup: %v)", resultErr, checkErr)
			return
		}
		if !active {
			return
		}
		if abortErr := d.AbortCherryPick(p.FeatureWorktree); abortErr != nil {
			resultErr = fmt.Errorf("%w (also failed aborting owned cherry-pick: %v)", resultErr, abortErr)
		}
	}()
	p.logTicketEvent(string(events.ConflictHit), pane, tab, sessionID, p.FeatureWorktree)
	p.Sink.ConflictResolutionStarted(p.Ticket.Identifier)

	// resolveCherryPickConflict itself corroborates against the sequencer
	// before returning success (see its doc comment), so a nil error here
	// already means the cherry-pick sequence is genuinely clear.
	resolutionSessionID, err = resolveCherryPickConflict(d, p)
	if err != nil {
		return LandResult{}, "", err
	}

	res, err = stampLanded(ld, lp)
	return res, resolutionSessionID, err
}

// foreignCherryPick reports whether the cherry-pick in progress in the feature
// worktree picks a commit no iteration branch of this epic holds. Anything it
// cannot tell (no CHERRY_PICK_HEAD, unreadable epic) counts as gx's own, the
// behaviour before this check existed.
func foreignCherryPick(d Deps, p iterationParams) (foreign bool, sha string) {
	sha, err := d.RevParse(p.FeatureWorktree, "CHERRY_PICK_HEAD")
	if err != nil || sha == "" || d.IsAncestor == nil {
		return false, sha
	}
	epics, err := tickets.Load(p.ScratchDir)
	if err != nil {
		return false, sha
	}
	for _, e := range epics {
		if e.Name != p.FeatureBranch {
			continue
		}
		for _, t := range e.Tickets {
			if ok, err := d.IsAncestor(p.FeatureWorktree, sha, iterBranch(p.FeatureBranch, t.Identifier)); err == nil && ok {
				return false, sha
			}
		}
		return true, sha
	}
	return false, sha
}

// landParamsFor maps an iteration's landing onto LandParams. The recorded SHA
// (ladder rung one) is best-effort: an unreadable run log just skips that rung.
func landParamsFor(p iterationParams, base, branch, sessionID string) LandParams {
	recorded := ""
	if events, ok, err := ReadEvents(p.ScratchDir, p.FeatureBranch); err == nil && ok {
		recorded = latestCherryPickedSHA(events, p.Ticket.Identifier)
	}
	return LandParams{
		FeatureWorktree: p.FeatureWorktree,
		FeatureBranch:   p.FeatureBranch,
		TicketID:        p.Ticket.Identifier,
		TicketPath:      p.Ticket.Path,
		SourceRange:     SourceRange{Base: base, Tip: branch},
		RecordedSHA:     recorded,
		Session: LandSession{
			Agent: p.Agent,
			Cwd:   iterationWorktreePath(p.WorktreeDir, p.FeatureBranch, p.Ticket.Identifier),
			ID:    sessionID,
		},
	}
}

// errConflictResolutionUnresolved is resolveCherryPickConflict's sentinel for
// a conflict-resolution failure caught by its own sequencer corroboration
// (as opposed to a plain launch/wait error, e.g. a stuck agent, which is
// reported the normal way). The failure is already recorded — needs-repair —
// on the conflict-resolution child ticket by the time this is returned, so
// finishIteration must swallow it rather than let it fall into the generic
// per-iteration error path, which would wrongly also mark the parent
// iteration ticket needs-repair (see ticket conflict-lifecycle/03: failures
// here belong to the child, not the parent).
var errConflictResolutionUnresolved = errors.New("conflict-resolution child ticket needs repair")

// findLiveConflictResolutionTab reports whether a tab labeled
// conflictLabel(p.Ticket.Identifier) is currently live in p.WorkspaceID —
// evidence that a conflict-resolution agent forked for this exact cherry-pick
// survived a crash/restart and still owns the sequencer state
// cherryPickWithConflictResolution is about to inspect.
func findLiveConflictResolutionTab(d Deps, p iterationParams) (session agentrunner.Session, found bool, err error) {
	session, found, err = d.Runner.Find(conflictLabel(p.Ticket.Identifier))
	if err != nil {
		return agentrunner.Session{}, false, fmt.Errorf("looking for a live conflict-resolution resolver for %s: %w", p.Ticket.Identifier, err)
	}
	return session, found, nil
}

// findConflictResolutionChildTicket locates the still-claimed
// conflict-resolution ticket forkConflictResolutionTicket created for
// p.Ticket — the durable record reattachLiveConflictResolver marks done once
// the live resolver it reattached to finishes.
func findConflictResolutionChildTicket(p iterationParams) (string, error) {
	epics, err := tickets.Load(p.ScratchDir)
	if err != nil {
		return "", fmt.Errorf("loading epics to find conflict-resolution child for %s: %w", p.Ticket.Identifier, err)
	}
	for _, epic := range epics {
		if epic.Name != p.FeatureBranch {
			continue
		}
		for _, t := range epic.Tickets {
			if t.Type != string(schema.TypeConflictResolution) {
				continue
			}
			if t.Parent == nil || *t.Parent != p.Ticket.Identifier {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(t.Status), string(schema.StatusClaimed)) {
				return t.Path, nil
			}
		}
	}
	return "", fmt.Errorf("no claimed conflict-resolution child ticket found for %s", p.Ticket.Identifier)
}

// reattachLiveConflictResolver waits out a conflict-resolution agent that
// survived a restart in liveTab, instead of aborting the cherry-pick
// sequencer state it still owns and forking a second resolver under the same
// tab label (see cherryPickWithConflictResolution's call site). It mirrors
// resolveCherryPickConflict's success path — locate the claimed child ticket,
// wait for its agent, mark it done, close its tab — but never creates a tab
// or child ticket of its own, since both already exist from before the
// crash.
func reattachLiveConflictResolver(d Deps, p iterationParams, live agentrunner.Session) (sessionID string, resultErr error) {
	label := conflictLabel(p.Ticket.Identifier)

	childPath, err := findConflictResolutionChildTicket(p)
	if err != nil {
		return "", err
	}

	agent, err := d.Runner.Status(live)
	if err != nil {
		return "", fmt.Errorf("conflict-resolution session %s: %w", label, err)
	}
	sessionID = agent.SessionID

	launchParams := p.launchAndPromptParams(label, live.ID, iterationTabID(d, label), "", p.FeatureWorktree, "", "")
	launchParams.FinishTimeoutMs = conflictResolutionTimeoutMs
	if !alreadyFinished(string(agent.State)) {
		if err := waitForFinish(d, launchParams, sessionID); err != nil {
			return "", fmt.Errorf("waiting for reattached conflict-resolution agent %s to finish: %w", label, err)
		}
	}

	inProgress, err := d.CherryPickInProgress(p.FeatureWorktree)
	if err != nil {
		return "", fmt.Errorf("checking cherry-pick state onto %s after reattached resolution: %w", p.FeatureBranch, err)
	}
	if inProgress {
		return "", fmt.Errorf("cherry-pick onto %s still in progress after reattached conflict-resolution agent %s finished", p.FeatureBranch, label)
	}

	if err := d.Runner.Stop(live); err != nil {
		return "", fmt.Errorf("closing reattached conflict-resolution session: %w", err)
	}

	if err := MarkDone(childPath); err != nil {
		return "", fmt.Errorf("marking conflict-resolution ticket %s done: %w", childPath, err)
	}

	return sessionID, nil
}

// resolveCherryPickConflict forks p.Ticket into a claimed conflict-resolution
// child ticket (see forkConflictResolutionTicket), then starts a fresh Runner
// session in the feature worktree and drives a "/gx-resolving-merge-conflicts"
// agent to completion in it, returning its agent session id. The iteration's
// own worktree/session are untouched while this runs. The idle signal is not
// trusted on its own — see the 2.1.228 incident, where herdr misdetected the
// pane as idle while the agent was still working — so once the wait reports
// the agent done, the child is only marked done after
// corroborating against the git sequencer itself (the ground truth for
// whether the resolution actually finished). A sequencer that's still
// conflicted at that point means the "done" signal was premature; the child
// ticket is marked needs-repair instead of done, and this returns
// errConflictResolutionUnresolved so the failure isn't also blamed on the
// parent iteration ticket.
func resolveCherryPickConflict(d Deps, p iterationParams) (sessionID string, resultErr error) {
	childPath, childID, err := forkConflictResolutionTicket(p)
	if err != nil {
		return "", fmt.Errorf("forking conflict-resolution ticket: %w", err)
	}

	label := conflictLabel(p.Ticket.Identifier)
	prompt := skillPrompt(p.Agent, "gx-resolving-merge-conflicts", "")

	l, err := startAndPrompt(d.Runner, agentrunner.StartOptions{
		Label: label,
		Epic:  p.FeatureBranch,
		Cwd:   p.FeatureWorktree,
		Kind:  agentrunner.Kind(p.Agent),
		Args:  agentArgs(p.Agent, p.ScratchDir, p.FeatureBranch, p.Model, p.Effort),
	}, prompt, func(attempt int, kind events.Kind, err error) {
		p.logLaunchFailed(label, attempt, kind, err)
	})
	var launchFail *launchFailure
	if errors.As(err, &launchFail) {
		return "", fmt.Errorf("starting conflict-resolution agent %s: %w", label, err)
	}
	defer func() {
		if stopErr := d.Runner.Stop(l.Session); stopErr != nil {
			if resultErr != nil {
				resultErr = fmt.Errorf("%w (also failed stopping conflict-resolution agent: %v)", resultErr, stopErr)
			} else {
				resultErr = fmt.Errorf("stopping conflict-resolution agent: %w", stopErr)
			}
		}
	}()

	launchParams := p.launchAndPromptParams(label, l.ID, "", prompt, p.FeatureWorktree, "", "")
	launchParams.Session = l.Session
	launchParams.FinishTimeoutMs = conflictResolutionTimeoutMs
	switch {
	case err != nil:
		err = fmt.Errorf("sending initial prompt: %w", err)
	case l.Adopted:
		sessionID, err = adoptedLaunch(d, launchParams)
	default:
		sessionID, err = promptedLaunch(d, launchParams, l.Baseline)
	}
	if err != nil {
		return "", fmt.Errorf("conflict-resolution agent %s did not finish (possibly stuck): %w", label, err)
	}

	// launchAndPrompt reports the agent done — corroborate against the git
	// sequencer itself before trusting that as "the resolution actually
	// finished" (see errConflictResolutionUnresolved's doc comment).
	inProgress, cpErr := d.CherryPickInProgress(p.FeatureWorktree)
	if cpErr != nil {
		reason := fmt.Sprintf("checking cherry-pick state after conflict-resolution agent %s reported done: %v", label, cpErr)
		return "", parkConflictResolutionChildNeedsRepair(p, childPath, childID, label, reason)
	}
	if inProgress {
		reason := fmt.Sprintf("conflict-resolution agent %s reported done, but the cherry-pick sequencer is still conflicted onto %s", label, p.FeatureBranch)
		return "", parkConflictResolutionChildNeedsRepair(p, childPath, childID, label, reason)
	}

	if err := MarkDone(childPath); err != nil {
		return "", fmt.Errorf("marking conflict-resolution ticket %s done: %w", childPath, err)
	}

	return sessionID, nil
}

// parkConflictResolutionChildNeedsRepair marks the conflict-resolution child
// ticket (not the parent iteration ticket — see errConflictResolutionUnresolved)
// needs-repair with reason, reports it, and returns the sentinel error that
// tells finishIteration to swallow this failure rather than also blaming the
// parent.
func parkConflictResolutionChildNeedsRepair(p iterationParams, childPath, childID, label, reason string) error {
	state := schema.NeedsRepairState{
		Label:    label,
		Branch:   p.FeatureBranch,
		Worktree: p.FeatureWorktree,
	}
	park(p.Sink, parkRequest{
		ScratchDir: p.ScratchDir, EpicName: p.FeatureBranch, Ticket: childID, Path: childPath,
		Type: events.NeedsRepair, Kind: events.IterationError, Reason: reason, Repair: state,
	})
	return fmt.Errorf("%w: %s: %s", errConflictResolutionUnresolved, childID, reason)
}
