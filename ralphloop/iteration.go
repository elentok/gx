package ralphloop

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
)

// conflictResolutionTimeoutMs bounds how long a conflict-resolution agent may
// run before it's treated as stuck, so a hung resolution surfaces as a
// distinct, actionable error instead of hanging the whole loop forever.
const conflictResolutionTimeoutMs = 30 * 60 * 1000

// runIteration drives one ticket through the full iteration lifecycle:
// create its worktree, launch and prompt the agent, wait for it to finish,
// cherry-pick its commits onto the feature branch, mark the ticket done, and
// remove the iteration worktree. If the agent finishes without landing any
// commits, the ticket is marked needs-info instead and the worktree/tab are
// left in place for inspection.
func runIteration(d Deps, p iterationParams) error {
	label := iterLabel(p.Ticket.Identifier)
	branch := iterBranch(p.FeatureBranch, p.Ticket.Identifier)
	path := filepath.Join(p.WorktreeDir, label)

	base, err := d.RevParse(p.FeatureWorktree, p.FeatureBranch)
	if err != nil {
		return fmt.Errorf("resolving %s tip: %w", p.FeatureBranch, err)
	}

	if err := d.AddWorktree(p.RepoDir, path, branch, base); err != nil {
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

	return finishIteration(d, p, path, tab.RootPaneID, tab.TabID, base, branch, sessionID)
}

// reattachIteration resumes a ticket left `Status: claimed` by a prior
// crashed/killed invocation whose iter-NN worktree/tab is still alive: it
// recomputes the worktree's deterministic path and finds its still-live tab
// (rather than creating either), skips straight to re-entering the "wait for
// the agent to finish" step (no launch or initial prompt — the agent may
// already be mid-turn or already done), then continues through the same
// cherry-pick/mark-done/remove completion path as a fresh iteration. The
// tab's agent is targeted by name (the iteration label, which AgentStart
// registered it under on the prior invocation) since no fresh AgentStart ran
// here to hand back a pane id. The original base commit is recovered via
// merge-base against the feature branch rather than the feature branch's
// current tip, since the feature branch may have advanced past this
// iteration's original branch point while the prior invocation was down.
func reattachIteration(d Deps, p iterationParams) error {
	label := iterLabel(p.Ticket.Identifier)
	branch := iterBranch(p.FeatureBranch, p.Ticket.Identifier)
	path := filepath.Join(p.WorktreeDir, label)

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

	base, err := d.MergeBase(path, branch, p.FeatureBranch)
	if err != nil {
		return fmt.Errorf("resolving %s's original base: %w", branch, err)
	}

	// No AgentStart was made this invocation, so there's no fresh
	// agent_session id to check smart-zone occupancy against, or to attach to
	// this ticket's later needs-info/cherry-picked events; the reattached
	// wait simply re-observes idle/done without that guardrail. StartEvent is
	// left empty for the same reason (no fresh launch happened here to log).
	launchParams := p.launchAndPromptParams(label, label, tabID, "", path, "", eventIterationFinished)
	if alreadyFinished(tab.AgentStatus) {
		// tab's status came from TabList just above, taken at reattach time —
		// the agent may have finished while the previous invocation was down,
		// with no further status transition ever coming, so waitForFinish's
		// AgentWait poll would have nothing new to observe.
		if p.Report != nil {
			p.Report("%s already finished at reattach; skipping wait\n", label)
		}
		launchParams.logLifecycleEvent(launchParams.FinishEvent, "")
	} else if err := waitForFinish(d, launchParams, ""); err != nil {
		return fmt.Errorf("waiting for reattached agent %s to finish: %w", label, err)
	}
	if strings.EqualFold(strings.TrimSpace(p.Ticket.Status), "needs-attention") {
		if err := Claim(p.Ticket.Path); err != nil {
			return fmt.Errorf("restoring ticket to claimed: %w", err)
		}
		p.Gate.ForceResume(label)
		p.Sink.IterationResumed(label, PauseNeedsAttention)
		if p.Report != nil {
			p.Report("resumed %s after restart recheck\n", label)
		}
		launchParams.logLifecycleEvent(eventResumed, "")
	}

	return finishIteration(d, p, path, label, tabID, base, branch, "")
}

// finishIteration lands a finished iteration's commits (or marks it
// needs-info if it produced none), then removes its worktree/tab on success
// — the shared tail of both the fresh (runIteration) and reattached
// (reattachIteration) iteration lifecycles. sessionID is the iteration
// agent's Claude Code session (empty for a reattached iteration, which made
// no fresh AgentStart call this invocation), attached to the needs-info/
// cherry-picked events logged here. pane is either the iteration's real pane
// id (fresh) or its agent name (reattached) — either is a valid herdr agent
// target.
func finishIteration(d Deps, p iterationParams, path, pane, tab, base, branch, sessionID string) error {
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
		// The agent finished without landing any commits: leave the worktree/
		// tab in place for inspection instead of silently marking done or
		// retrying, and let the scheduler move on to other unblocked tickets.
		if err := MarkNeedsInfo(p.Ticket.Path); err != nil {
			return fmt.Errorf("marking ticket needs-info: %w", err)
		}
		p.logTicketEvent(eventNeedsInfo, pane, tab, sessionID, path)
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

	return finishCleanup(d, p.RepoDir, p.FeatureWorktree, path, branch, tab)
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
	return MarkDoneWithMetadata(p.Ticket.Path, occupancy, sessionID)
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
	return MarkDoneWithMetadata(p.Ticket.Path, occupancy, sessionID)
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

	if err := cherryPickWithConflictResolution(d, p, base, branch, sessionID, pane, tab); err != nil {
		return "", err
	}

	iterationCwd := filepath.Join(p.WorktreeDir, iterLabel(p.Ticket.Identifier))
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

	return landedSHA, nil
}

// finishCleanup removes an iteration's now-redundant worktree, tab, and
// branch. Each check is independent since callers run this against state left
// in different shapes: a normal completion (worktree/tab/branch all just
// created and definitely present), or a done ticket's leftover state found on
// startup after a crash (any subset of the three may have survived — see
// classifyDoneTicket's doneStaleCleanup). tabID is "" if no live tab was
// found for this iteration.
func finishCleanup(d Deps, repoDir, featureWorktree, path, branch, tabID string) error {
	hasWorktree, err := d.WorktreeExists(path)
	if err != nil {
		return fmt.Errorf("checking iteration worktree: %w", err)
	}
	if hasWorktree {
		if err := d.RemoveWorktree(repoDir, path, true); err != nil {
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
// sends "/resolving-merge-conflicts", and waits for that agent to finish
// before confirming the cherry-pick sequence completed. sessionID/pane/tab
// attach the iteration agent's own session to the conflict-hit event (the
// resolution agent hasn't launched yet at that point); conflict-resolved is
// instead attributed to the resolution agent's own distinct session, so its
// cost/occupancy are counted too rather than silently dropped from the
// report.
func cherryPickWithConflictResolution(d Deps, p iterationParams, base, branch, sessionID, pane, tab string) error {
	p.Sink.CherryPickStarted(p.Ticket.Identifier)
	pickErr := d.CherryPickRange(p.FeatureWorktree, base, branch)
	if pickErr == nil {
		return nil
	}

	inProgress, err := d.CherryPickInProgress(p.FeatureWorktree)
	if err != nil {
		return fmt.Errorf("checking cherry-pick state onto %s: %w", p.FeatureBranch, err)
	}
	if !inProgress {
		return fmt.Errorf("cherry-picking onto %s: %w", p.FeatureBranch, pickErr)
	}
	p.logTicketEvent(eventConflictHit, pane, tab, sessionID, p.FeatureWorktree)
	p.Sink.ConflictResolutionStarted(p.Ticket.Identifier)

	resolutionSessionID, err := resolveCherryPickConflict(d, p)
	if err != nil {
		return err
	}

	inProgress, err = d.CherryPickInProgress(p.FeatureWorktree)
	if err != nil {
		return fmt.Errorf("checking cherry-pick state onto %s after resolution: %w", p.FeatureBranch, err)
	}
	if inProgress {
		return fmt.Errorf("cherry-pick onto %s still in progress after conflict-resolution agent finished", p.FeatureBranch)
	}
	p.logTicketEvent(eventConflictResolved, "", "", resolutionSessionID, p.FeatureWorktree)
	return nil
}

// resolveCherryPickConflict launches a fresh pane in the feature worktree and
// drives a "/resolving-merge-conflicts" agent to completion in it, returning
// its agent session id. The iteration's own worktree/tab are untouched
// while this runs.
func resolveCherryPickConflict(d Deps, p iterationParams) (string, error) {
	label := conflictLabel(p.Ticket.Identifier)

	tab, err := d.TabCreate(herdr.TabCreateOptions{
		WorkspaceID: p.WorkspaceID,
		Cwd:         p.FeatureWorktree,
		Label:       label,
	})
	if err != nil {
		return "", fmt.Errorf("creating conflict-resolution pane: %w", err)
	}

	launchParams := p.launchAndPromptParams(label, tab.RootPaneID, tab.TabID, skillPrompt(p.Agent, "resolving-merge-conflicts", ""), p.FeatureWorktree, "", "")
	launchParams.FinishTimeoutMs = conflictResolutionTimeoutMs
	sessionID, err := launchAndPrompt(d, launchParams)
	if err != nil {
		return "", fmt.Errorf("conflict-resolution agent %s did not finish (possibly stuck): %w", label, err)
	}

	return sessionID, nil
}
