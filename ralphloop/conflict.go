package ralphloop

import (
	"context"
	"errors"
	"fmt"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/supervise"
	"github.com/elentok/gx/ralphloop/ticketops"
	"strings"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/merge"
	"github.com/elentok/gx/ralphloop/runlog"

	"github.com/elentok/gx/ralphloop/worktree"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// conflictResolutionTimeoutMs bounds how long a conflict-resolution agent may
// run before it's treated as stuck, so a hung resolution surfaces as a
// distinct, actionable error instead of hanging the whole loop forever.
const conflictResolutionTimeoutMs = 30 * 60 * 1000

// cherryPickWithConflictResolution cherry-picks base..branch onto
// p.FeatureWorktree. On a conflict, it launches a fresh pane in the feature
// worktree (where the conflict markers are, not the iteration worktree),
// sends "/gx-resolving-merge-conflicts", and waits for that agent to finish
// before confirming the cherry-pick sequence completed. sessionID/pane/tab
// attach the iteration agent's own session to the conflict-hit event (the
// resolution agent hasn't launched yet at that point). On resolution it
// returns that agent's distinct session so mergeCherryPick can emit
// conflict-resolved with the final post-trailer SHA. The returned result is
// merge.Merged (stamped) or merge.AlreadyApplied, never merge.Conflicted.
func cherryPickWithConflictResolution(d deps.Deps, p iterationParams, base, branch, sessionID, pane string) (res merge.MergeResult, resolutionSessionID string, resultErr error) {
	ld, lp := merge.MergeDepsOf(d), mergeParamsFor(p, base, branch, sessionID)
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
		return merge.MergeResult{}, "", fmt.Errorf("checking for stale cherry-pick onto %s: %w", p.FeatureBranch, err)
	}
	if inProgress {
		liveSession, found, err := findLiveConflictResolutionTab(d, p)
		if err != nil {
			return merge.MergeResult{}, "", err
		}
		if found {
			resolutionSessionID, err := reattachLiveConflictResolver(d, p, liveSession)
			if err != nil {
				return merge.MergeResult{}, "", err
			}
			res, err := merge.StampMerged(ld, lp)
			return res, resolutionSessionID, err
		}
		// A merge marker means the sequencer state is a human's pending
		// conflict, not crash debris: aborting would discard their work.
		lockDir, err := mergeLockDirFor(d, p.ScratchDir, p.FeatureBranch)
		if err != nil {
			return merge.MergeResult{}, "", err
		}
		marker, err := merge.ReadMergeMarker(lockDir)
		if err != nil {
			return merge.MergeResult{}, "", fmt.Errorf("reading merge marker: %w", err)
		}
		if marker != nil {
			return merge.MergeResult{}, "", errMergeDeferred
		}
		// Only gx's own debris is lossless to abort: a pick of some iteration
		// branch's commit. Anything else is a person's work in progress.
		if foreign, sha := foreignCherryPick(d, p); foreign {
			return merge.MergeResult{}, "", fmt.Errorf("the feature worktree %s has a cherry-pick of %s in progress that gx did not start; finish or abort it, then merge again", p.FeatureWorktree, sha)
		}
		if err := d.AbortCherryPick(p.FeatureWorktree); err != nil {
			return merge.MergeResult{}, "", fmt.Errorf("aborting stale cherry-pick onto %s: %w", p.FeatureBranch, err)
		}
	}

	// MergeTicket recognizes an iteration whose patch concurrent work already
	// made redundant (git would stop that as an empty cherry-pick and leave
	// sequencer state behind) before touching the worktree.
	res, err = merge.MergeTicket(ld, lp)
	if err != nil {
		return merge.MergeResult{}, "", err
	}
	if res.Outcome != merge.Conflicted {
		return res, "", nil
	}
	// This call now owns the active sequencer state. Always clean it up on
	// failure, otherwise the next merging serialized behind this one could
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
	p.logTicketEvent(string(events.ConflictHit), pane, sessionID, p.FeatureWorktree)
	p.Sink.ConflictResolutionStarted(p.Ticket.Identifier)

	// resolveCherryPickConflict itself corroborates against the sequencer
	// before returning success (see its doc comment), so a nil error here
	// already means the cherry-pick sequence is genuinely clear.
	resolutionSessionID, err = resolveCherryPickConflict(d, p)
	if err != nil {
		return merge.MergeResult{}, "", err
	}

	res, err = merge.StampMerged(ld, lp)
	return res, resolutionSessionID, err
}

// foreignCherryPick reports whether the cherry-pick in progress in the feature
// worktree picks a commit no iteration branch of this epic holds. Anything it
// cannot tell (no CHERRY_PICK_HEAD, unreadable epic) counts as gx's own, the
// behaviour before this check existed.
func foreignCherryPick(d deps.Deps, p iterationParams) (foreign bool, sha string) {
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
			if ok, err := d.IsAncestor(p.FeatureWorktree, sha, worktree.IterationBranch(p.FeatureBranch, t.Identifier)); err == nil && ok {
				return false, sha
			}
		}
		return true, sha
	}
	return false, sha
}

// mergeParamsFor maps an iteration's merging onto MergeParams. The recorded SHA
// (ladder rung one) is best-effort: an unreadable run log just skips that rung.
func mergeParamsFor(p iterationParams, base, branch, sessionID string) merge.MergeParams {
	recorded := ""
	if events, ok, err := runlog.ReadEvents(p.ScratchDir, p.FeatureBranch); err == nil && ok {
		recorded = merge.LatestCherryPickedSHA(events, p.Ticket.Identifier)
	}
	return merge.MergeParams{
		FeatureWorktree: p.FeatureWorktree,
		FeatureBranch:   p.FeatureBranch,
		TicketID:        p.Ticket.Identifier,
		TicketPath:      p.Ticket.Path,
		SourceRange:     merge.SourceRange{Base: base, Tip: branch},
		RecordedSHA:     recorded,
		Session: merge.MergeSession{
			Agent: p.Agent,
			Cwd:   worktree.IterationWorktreePath(p.WorktreeDir, p.FeatureBranch, p.Ticket.Identifier),
			ID:    sessionID,
		},
	}
}

// errConflictResolutionUnresolved is resolveCherryPickConflict's sentinel for
// a conflict-resolution failure caught by its own sequencer corroboration
// (as opposed to a plain launch/wait error, e.g. a stuck agent, which is
// reported the normal way). The failure is already recorded — paused (repair) —
// on the conflict-resolution child ticket by the time this is returned, so
// finishIteration must swallow it rather than let it fall into the generic
// per-iteration error path, which would wrongly also mark the parent
// iteration ticket paused (repair) (see ticket conflict-lifecycle/03: failures
// here belong to the child, not the parent).
var errConflictResolutionUnresolved = errors.New("conflict-resolution child ticket paused for repair")

// findLiveConflictResolutionTab reports whether a tab labeled
// conflictLabel(p.Ticket.Identifier) is currently live in p.WorkspaceID —
// evidence that a conflict-resolution agent forked for this exact cherry-pick
// survived a crash/restart and still owns the sequencer state
// cherryPickWithConflictResolution is about to inspect.
func findLiveConflictResolutionTab(d deps.Deps, p iterationParams) (session agentrunner.Session, found bool, err error) {
	session, found, err = d.Runner.Find(worktree.ConflictLabel(p.Ticket.Identifier))
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
func reattachLiveConflictResolver(d deps.Deps, p iterationParams, live agentrunner.Session) (sessionID string, resultErr error) {
	label := worktree.ConflictLabel(p.Ticket.Identifier)

	childPath, err := findConflictResolutionChildTicket(p)
	if err != nil {
		return "", err
	}

	agent, err := d.Runner.Status(live)
	if err != nil {
		return "", fmt.Errorf("conflict-resolution session %s: %w", label, err)
	}
	sessionID = agent.SessionID

	launchParams := p.launchAndPromptParams(label, live.ID, "", p.FeatureWorktree, "", "")
	launchParams.FinishTimeoutMs = conflictResolutionTimeoutMs
	if !supervise.AlreadyFinished(string(agent.State)) {
		if err := supervise.WaitForFinish(d, launchParams, sessionID); err != nil {
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

	if err := ticketops.MarkChildDone(childPath, sessionID); err != nil {
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
// ticket is marked paused (repair) instead of done, and this returns
// errConflictResolutionUnresolved so the failure isn't also blamed on the
// parent iteration ticket.
func resolveCherryPickConflict(d deps.Deps, p iterationParams) (sessionID string, resultErr error) {
	childPath, childID, err := forkConflictResolutionTicket(p)
	if err != nil {
		return "", fmt.Errorf("forking conflict-resolution ticket: %w", err)
	}

	label := worktree.ConflictLabel(p.Ticket.Identifier)
	prompt := agent.SkillPrompt(p.Agent, "gx-resolving-merge-conflicts", "")

	l, err := supervise.StartAndPromptLogged(context.Background(), d.Runner, agentrunner.StartOptions{
		Label: label,
		Epic:  p.FeatureBranch,
		Cwd:   p.FeatureWorktree,
		Kind:  agentrunner.Kind(p.Agent),
		Args:  agent.AgentArgs(p.Agent, p.FeatureWorktree, p.ScratchDir, p.FeatureBranch, p.Model, p.Effort),
	}, prompt, func(attempt int, kind events.Kind, err error) {
		p.logLaunchFailed(label, attempt, kind, err)
	}, func(attempt int, decision string) {
		p.logPromptNudged(label, attempt, decision)
	})

	var launchFail *supervise.LaunchFailure
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

	launchParams := p.launchAndPromptParams(label, l.ID, prompt, p.FeatureWorktree, "", "")
	launchParams.Session = l.Session
	launchParams.FinishTimeoutMs = conflictResolutionTimeoutMs
	switch {
	case err != nil:
		err = fmt.Errorf("sending initial prompt: %w", err)
	case l.Adopted:
		sessionID, err = supervise.AdoptedLaunch(d, launchParams)
	default:
		sessionID, err = supervise.PromptedLaunch(d, launchParams, l.Baseline)
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
		return "", pauseConflictResolutionChildRepair(p, childPath, childID, label, reason)
	}
	if inProgress {
		reason := fmt.Sprintf("conflict-resolution agent %s reported done, but the cherry-pick sequencer is still conflicted onto %s", label, p.FeatureBranch)
		return "", pauseConflictResolutionChildRepair(p, childPath, childID, label, reason)
	}

	if err := ticketops.MarkChildDone(childPath, sessionID); err != nil {
		return "", fmt.Errorf("marking conflict-resolution ticket %s done: %w", childPath, err)
	}

	return sessionID, nil
}

// pauseConflictResolutionChildRepair marks the conflict-resolution child
// ticket (not the parent iteration ticket — see errConflictResolutionUnresolved)
// paused (repair) with reason, reports it, and returns the sentinel error that
// tells finishIteration to swallow this failure rather than also blaming the
// parent.
func pauseConflictResolutionChildRepair(p iterationParams, childPath, childID, label, reason string) error {
	state := schema.FaultState{
		Label:    label,
		Branch:   p.FeatureBranch,
		Worktree: p.FeatureWorktree,
	}
	ticketops.PauseTicket(p.Sink, ticketops.PauseRequest{
		ScratchDir: p.ScratchDir, EpicName: p.FeatureBranch, Ticket: childID, Path: childPath,
		Type: events.Paused, Kind: events.IterationError, Reason: reason, Repair: state,
	})
	return fmt.Errorf("%w: %s: %s", errConflictResolutionUnresolved, childID, reason)
}
