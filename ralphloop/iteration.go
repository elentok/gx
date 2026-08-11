package ralphloop

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
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

	base, err := d.RevParse(p.FeatureWorktree, p.FeatureBranch)
	if err != nil {
		return fmt.Errorf("resolving %s tip: %w", p.FeatureBranch, err)
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
	p.logTicketEventReason(eventDepsInstalled, "", "", "", path, command)

	tab, err := d.TabCreate(herdr.TabCreateOptions{
		WorkspaceID: p.WorkspaceID,
		Cwd:         path,
		Label:       label,
	})
	if err != nil {
		return fmt.Errorf("opening iteration tab: %w", err)
	}

	prompt := skillPrompt(p.Agent, p.Skill, p.Ticket.Path)
	launchParams := p.launchAndPromptParams(label, tab.RootPaneID, tab.TabID, prompt, path, eventIterationStarted, eventIterationFinished)
	sessionID, err := launchAndPrompt(d, launchParams)
	if err != nil {
		return err
	}
	if sessionID != "" {
		if err := AppendSessionID(p.Ticket.Path, sessionID); err != nil {
			return fmt.Errorf("appending session id for ticket %s: %w", p.Ticket.Identifier, err)
		}
	}

	return finishIteration(d, p, path, tab.RootPaneID, tab.TabID, base, branch, sessionID)
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

	tabs, err := d.TabList(p.WorkspaceID)
	if err != nil {
		return fmt.Errorf("finding live tab for reattached iteration %s: %w", label, err)
	}
	var tab herdr.Tab
	for _, t := range tabs {
		if t.Label == label {
			tab = t
			break
		}
	}
	if tab.TabID == "" {
		return fmt.Errorf("no live tab found for reattached iteration %s", label)
	}
	tabID := tab.TabID
	agent, err := d.AgentGet(label)
	if err != nil {
		return fmt.Errorf("reading live agent state for reattached iteration %s: %w", label, err)
	}
	if agent.PaneID == "" || agent.TabID != tabID || agent.WorkspaceID != p.WorkspaceID {
		return fmt.Errorf("live agent state for reattached iteration %s does not match workspace %s and tab %s", label, p.WorkspaceID, tabID)
	}
	if p.Agent == AgentCodex {
		if agent.AgentSession == "" {
			return fmt.Errorf("missing live Codex session for reattached iteration %s; keep the tab open and retry after Herdr reports its session", label)
		}
		verified, verifyErr := d.VerifyCodexSession(path, agent.AgentSession)
		if verifyErr != nil {
			return fmt.Errorf("verifying live Codex session %s for reattached iteration %s: %w", agent.AgentSession, label, verifyErr)
		}
		if !verified {
			return fmt.Errorf("live Codex session %s for reattached iteration %s does not match rollout metadata for cwd %s", agent.AgentSession, label, path)
		}
	}
	if agent.AgentSession != "" {
		if err := AppendSessionID(p.Ticket.Path, agent.AgentSession); err != nil {
			return fmt.Errorf("appending session id for reattached ticket %s: %w", p.Ticket.Identifier, err)
		}
	}

	base, err := d.MergeBase(path, branch, p.FeatureBranch)
	if err != nil {
		return fmt.Errorf("resolving %s's original base: %w", branch, err)
	}

	// StartEvent remains empty because reattachment must not imply a fresh
	// launch; all later events use the recovered native session identity.
	launchParams := p.launchAndPromptParams(label, agent.PaneID, tabID, "", path, "", eventIterationFinished)
	if alreadyFinished(agent.AgentStatus) {
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
		launchParams.logLifecycleEvent(launchParams.FinishEvent, agent.AgentSession)
	} else if err := waitForFinish(d, launchParams, agent.AgentSession); err != nil {
		return fmt.Errorf("waiting for reattached agent %s to finish: %w", label, err)
	}
	if strings.EqualFold(strings.TrimSpace(p.Ticket.Status), "needs-repair") {
		if err := Claim(p.Ticket.Path); err != nil {
			return fmt.Errorf("restoring ticket to claimed: %w", err)
		}
		p.Gate.ForceResume(label)
		p.Sink.IterationResumed(label, PauseNeedsRepair)
		if p.Report != nil {
			p.Report("resumed %s after restart recheck\n", label)
		}
		launchParams.logLifecycleEvent(eventResumed, agent.AgentSession)
	}

	return finishIteration(d, p, path, agent.PaneID, tabID, base, branch, agent.AgentSession)
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
		return nil
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
		current, err := schema.ParseTicket(p.Ticket.Path)
		if err != nil {
			return fmt.Errorf("reading ticket %s for commitless check: %w", p.Ticket.Path, err)
		}
		if current.IsCommitless() && current.IterationStatus == schema.IterationStatusFinished {
			stampCommitlessMetrics(p, path, sessionID)
			if err := MarkDone(p.Ticket.Path); err != nil {
				return fmt.Errorf("marking commitless ticket %s done: %w", p.Ticket.Identifier, err)
			}
			p.logTicketEvent(eventCommitless, pane, tab, sessionID, path)
			return finishCleanup(d, p.WorktreeLock, p.RepoDir, p.FeatureWorktree, path, branch, tab)
		}

		// The agent finished without landing any commits: leave the worktree/
		// tab in place for inspection instead of silently marking done or
		// retrying, and let the scheduler move on to other unblocked tickets.
		if err := MarkNeedsAnswer(p.Ticket.Path); err != nil {
			return fmt.Errorf("marking ticket needs-answer: %w", err)
		}
		p.logTicketEvent(eventNeedsAnswer, pane, tab, sessionID, path)
		p.Sink.TicketNeedsAnswer(p.Ticket.Identifier, p.FeatureBranch)
		return nil
	}

	landedSHA, err := landCherryPick(d, p, base, branch, sessionID, pane, tab)
	if err != nil {
		return err
	}
	p.logTicketEventSHA(eventCherryPicked, pane, tab, sessionID, path, "", landedSHA)

	if err := markDoneStampingCloseMetadata(d, p, path, sessionID); err != nil {
		return fmt.Errorf("marking ticket done: %w", err)
	}

	return finishCleanup(d, p.WorktreeLock, p.RepoDir, p.FeatureWorktree, path, branch, tab)
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

	if err := MarkNeedsAnswer(p.Ticket.Path); err != nil {
		return false, fmt.Errorf("adopting needs-answer report: %w", err)
	}
	p.logTicketEvent(eventNeedsAnswer, pane, tab, sessionID, path)
	p.Sink.TicketNeedsAnswer(p.Ticket.Identifier, p.FeatureBranch)
	return true, nil
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
	events, ok, err := readEvents(p.ScratchDir, p.FeatureBranch)
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
	events, ok, err := readEvents(p.ScratchDir, p.FeatureBranch)
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
// completion (finishIteration) and a startup repair's re-cherry-pick
// (repairRecoverableTicket). p.FeatureLock is held for the duration since
// this mutates the shared feature worktree's working directory; the landed
// SHA is resolved before releasing it, before another iteration's cherry-pick
// can advance the tip past this one's — otherwise the recorded SHA could
// belong to a different ticket entirely. Before resolving the SHA, it writes
// the landing iteration's session metrics (actual_context_window,
// elapsed_time) into the ticket's frontmatter via writeLandedMetrics, then
// stamps ticketTrailerKey and — when those metrics were available — the same
// values onto tokensTrailerKey/elapsedTrailerKey, all in a single amend
// (Deps.AppendTrailers) rather than one amend per trailer.
func landCherryPick(d Deps, p iterationParams, base, branch, sessionID, pane, tab string) (string, error) {
	p.FeatureLock.Lock()
	defer p.FeatureLock.Unlock()

	picked, resolutionSessionID, err := cherryPickWithConflictResolution(d, p, base, branch, sessionID, pane, tab)
	if err != nil {
		return "", err
	}
	if !picked {
		landedSHA, err := d.RevParse(p.FeatureWorktree, "HEAD")
		if err != nil {
			return "", fmt.Errorf("resolving already-applied commit on %s: %w", p.FeatureBranch, err)
		}
		return landedSHA, nil
	}

	iterationCwd := iterationWorktreePath(p.WorktreeDir, p.FeatureBranch, p.Ticket.Identifier)
	contextWindow, elapsedSeconds, hasMetrics, err := writeLandedMetrics(p.Agent, iterationCwd, sessionID, p.Ticket.Path)
	if err != nil {
		return "", fmt.Errorf("writing landed metrics for ticket %s: %w", p.Ticket.Identifier, err)
	}

	trailers := []git.Trailer{{Key: ticketTrailerKey, Value: ticketTrailerValue(p.FeatureBranch, p.Ticket.Identifier)}}
	if hasMetrics {
		trailers = append(trailers,
			git.Trailer{Key: tokensTrailerKey, Value: strconv.Itoa(contextWindow)},
			git.Trailer{Key: elapsedTrailerKey, Value: strconv.Itoa(elapsedSeconds) + "s"},
		)
	}
	if err := d.AppendTrailers(p.FeatureWorktree, trailers...); err != nil {
		return "", fmt.Errorf("stamping trailers on landed commit: %w", err)
	}

	landedSHA, err := d.RevParse(p.FeatureWorktree, "HEAD")
	if err != nil {
		return "", fmt.Errorf("resolving landed commit on %s: %w", p.FeatureBranch, err)
	}
	if resolutionSessionID != "" {
		p.logTicketEventSHA(eventConflictResolved, "", "", resolutionSessionID, p.FeatureWorktree, "", landedSHA)
	}

	return landedSHA, nil
}

// finishCleanup removes an iteration's now-redundant worktree, tab, and
// branch. Each check is independent since callers run this against state left
// in different shapes: a normal completion (worktree/tab/branch all just
// created and definitely present), or a done ticket's leftover state found on
// startup after a crash (any subset of the three may have survived — see
// classifyDoneTicket's doneStaleCleanup). tabID is "" if no live tab was
// found for this iteration.
func finishCleanup(d Deps, worktreeLock *sync.Mutex, repoDir, featureWorktree, path, branch, tabID string) error {
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

	if tabID != "" {
		if err := d.TabClose(tabID); err != nil {
			return fmt.Errorf("closing iteration tab: %w", err)
		}
	}

	if branchExists(d, featureWorktree, branch) {
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
// conflict-resolved with the final post-trailer SHA.
func cherryPickWithConflictResolution(d Deps, p iterationParams, base, branch, sessionID, pane, tab string) (picked bool, resolutionSessionID string, resultErr error) {
	p.Sink.CherryPickStarted(p.Ticket.Identifier)

	// A previous invocation may have died after starting a cherry-pick but
	// before recording its outcome. Never let the next ticket inherit or
	// resolve that operation as its own: its iteration branch is durable, so
	// aborting here is lossless and lets reconciliation retry it explicitly.
	inProgress, err := d.CherryPickInProgress(p.FeatureWorktree)
	if err != nil {
		return false, "", fmt.Errorf("checking for stale cherry-pick onto %s: %w", p.FeatureBranch, err)
	}
	if inProgress {
		if err := d.AbortCherryPick(p.FeatureWorktree); err != nil {
			return false, "", fmt.Errorf("aborting stale cherry-pick onto %s: %w", p.FeatureBranch, err)
		}
	}

	// Concurrent work can make an iteration's entire patch redundant before
	// it lands. Git stops that as an empty cherry-pick and leaves sequencer
	// state behind, so recognize the safe no-op before touching the worktree.
	applied, err := d.PatchesApplied(p.FeatureWorktree, p.FeatureBranch, base, branch)
	if err != nil {
		return false, "", fmt.Errorf("checking whether ticket %s is already applied: %w", p.Ticket.Identifier, err)
	}
	if applied {
		return false, "", nil
	}

	pickErr := d.CherryPickRange(p.FeatureWorktree, base, branch)
	if pickErr == nil {
		return true, "", nil
	}

	inProgress, err = d.CherryPickInProgress(p.FeatureWorktree)
	if err != nil {
		return false, "", fmt.Errorf("checking cherry-pick state onto %s: %w", p.FeatureBranch, err)
	}
	if !inProgress {
		return false, "", fmt.Errorf("cherry-picking onto %s: %w", p.FeatureBranch, pickErr)
	}
	// This call now owns the active sequencer state. Always clean it before
	// FeatureLock is released on failure, otherwise later tickets can mistake
	// this ticket's CHERRY_PICK_HEAD for their own conflict.
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
	p.logTicketEvent(eventConflictHit, pane, tab, sessionID, p.FeatureWorktree)
	p.Sink.ConflictResolutionStarted(p.Ticket.Identifier)

	resolutionSessionID, err = resolveCherryPickConflict(d, p)
	if err != nil {
		return false, "", err
	}

	inProgress, err = d.CherryPickInProgress(p.FeatureWorktree)
	if err != nil {
		return false, "", fmt.Errorf("checking cherry-pick state onto %s after resolution: %w", p.FeatureBranch, err)
	}
	if inProgress {
		return false, "", fmt.Errorf("cherry-pick onto %s still in progress after conflict-resolution agent finished", p.FeatureBranch)
	}
	return true, resolutionSessionID, nil
}

// resolveCherryPickConflict launches a fresh pane in the feature worktree and
// drives a "/gx-resolving-merge-conflicts" agent to completion in it, returning
// its agent session id. The iteration's own worktree/tab are untouched
// while this runs.
func resolveCherryPickConflict(d Deps, p iterationParams) (sessionID string, resultErr error) {
	label := conflictLabel(p.Ticket.Identifier)

	tab, err := d.TabCreate(herdr.TabCreateOptions{
		WorkspaceID: p.WorkspaceID,
		Cwd:         p.FeatureWorktree,
		Label:       label,
	})
	if err != nil {
		return "", fmt.Errorf("creating conflict-resolution pane: %w", err)
	}
	defer func() {
		closeErr := d.TabClose(tab.TabID)
		if closeErr != nil {
			if resultErr != nil {
				resultErr = fmt.Errorf("%w (also failed closing conflict-resolution tab: %v)", resultErr, closeErr)
			} else {
				resultErr = fmt.Errorf("closing conflict-resolution tab: %w", closeErr)
			}
			return
		}
		// herdr's tab-close call can report success without the tab actually
		// disappearing (upstream bug, see .scratch/tickets-queue-polish/issues/
		// 04-conflict-resolution-tab-not-closing.md) — check for it so a false
		// success doesn't silently leak a tab, without turning it into a hard
		// failure that would take the whole ticket down over one leaked pane.
		tabs, listErr := d.TabList(p.WorkspaceID)
		if listErr != nil {
			return
		}
		for _, t := range tabs {
			if t.TabID == tab.TabID {
				log.Printf("conflict-resolution tab %s reported closed but is still present in tab list", tab.TabID)
				break
			}
		}
	}()

	launchParams := p.launchAndPromptParams(label, tab.RootPaneID, tab.TabID, skillPrompt(p.Agent, "gx-resolving-merge-conflicts", ""), p.FeatureWorktree, "", "")
	launchParams.FinishTimeoutMs = conflictResolutionTimeoutMs
	sessionID, err = launchAndPrompt(d, launchParams)
	if err != nil {
		return "", fmt.Errorf("conflict-resolution agent %s did not finish (possibly stuck): %w", label, err)
	}

	return sessionID, nil
}
