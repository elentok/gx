package ralphloop

import (
	"fmt"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
)

// repairRecoverableTicket re-lands a doneRecoverable ticket's commits: a
// prior crash left it marked done with its commits missing from the feature
// branch, but its iteration branch still holds them. It resolves the same
// base commit reattachIteration would (merge-base against the feature
// branch's current tip, since the tip may have advanced past this ticket's
// original branch point while the crashed run was down), then re-runs it
// through the exact cherry-pick-plus-conflict-resolution path a normal
// iteration's first cherry-pick uses — no separate repair-specific conflict
// handling. On success it logs the same cherry-picked event a normal
// iteration would, reports what was restored, and finishes whatever cleanup
// the crash left undone (leftover worktree/tab; branch deletion is a later
// ticket's job).
func repairRecoverableTicket(d Deps, rp reconcileParams, featureBranch string, t tickets.Ticket, tabs []herdr.Tab) error {
	paths := rp.Paths
	branch := iterBranch(featureBranch, t.Identifier)
	label := iterLabel(t.Identifier)
	path := iterationWorktreePath(paths.WorktreeDir, featureBranch, t.Identifier)

	base, err := d.MergeBase(paths.FeatureWorktree, branch, featureBranch)
	if err != nil {
		return fmt.Errorf("resolving %s's base for repair: %w", branch, err)
	}

	p := iterationParams{
		WorkspaceID:      rp.WorkspaceID,
		RepoDir:          paths.RepoDir,
		WorktreeDir:      paths.WorktreeDir,
		FeatureWorktree:  paths.FeatureWorktree,
		FeatureBranch:    featureBranch,
		Agent:            rp.Agent,
		Skill:            rp.Skill,
		Ticket:           t,
		ScratchDir:       paths.ScratchDir,
		FeatureLock:      rp.FeatureLock,
		WorktreeLock:     rp.WorktreeLock,
		SmartZone:        rp.SmartZone,
		Gate:             rp.Gate,
		ResumeSignalPath: rp.ResumeSignalPath,
		Sink:             rp.Sink,
	}

	// Seed a live row so landCherryPick's CherryPickStarted/
	// ConflictResolutionStarted events below have a live entry to update —
	// without this, the tickets tab shows a done ticket's stale disk status
	// while this repair runs.
	rp.Sink.TicketRecovering(t.Identifier)

	landedSHA, err := landCherryPick(d, p, base, branch, "", "", "")
	if err != nil {
		return fmt.Errorf("re-cherry-picking ticket %s during startup repair: %w", t.Identifier, err)
	}

	p.logTicketEventSHA(eventCherryPicked, "", "", "", path, "", landedSHA)
	rp.Sink.TicketRecovered(t.Identifier, featureBranch, branch, landedSHA)

	// Branch deletion is left to finishStaleCleanup/finishCleanup elsewhere:
	// this repair just re-landed the commits, so the branch that held them
	// stays until a later ticket cleans it up.
	tabID := tabIDForLabel(tabs, label)
	hasWorktree, err := d.WorktreeExists(path)
	if err != nil {
		return fmt.Errorf("checking leftover worktree during repair cleanup: %w", err)
	}
	if hasWorktree {
		rp.WorktreeLock.Lock()
		err := d.RemoveWorktree(paths.RepoDir, path, true)
		rp.WorktreeLock.Unlock()
		if err != nil {
			return fmt.Errorf("removing repaired iteration worktree: %w", err)
		}
	}
	if tabID != "" {
		if err := d.TabClose(tabID); err != nil {
			return fmt.Errorf("closing repaired iteration tab: %w", err)
		}
	}

	return nil
}

// finishStaleCleanup completes an interrupted cleanup for a done ticket whose
// commits already landed on the feature branch, but whose iteration
// worktree/tab/branch survived a crash that landed between marking done and
// the cleanup step right after it — the same tail finishIteration runs on the
// normal completion path.
func finishStaleCleanup(d Deps, rp reconcileParams, featureBranch string, t tickets.Ticket, tabs []herdr.Tab) error {
	paths := rp.Paths
	label := iterLabel(t.Identifier)
	branch := iterBranch(featureBranch, t.Identifier)
	path := iterationWorktreePath(paths.WorktreeDir, featureBranch, t.Identifier)
	tabID := tabIDForLabel(tabs, label)

	return finishCleanup(d, rp.WorktreeLock, paths.RepoDir, paths.FeatureWorktree, path, branch, tabID)
}

// tabIDForLabel finds the tab id of the live tab named label, or "" if none
// is live.
func tabIDForLabel(tabs []herdr.Tab, label string) string {
	for _, tab := range tabs {
		if tab.Label == label {
			return tab.TabID
		}
	}
	return ""
}

// markDoneTicketUnrecoverable flags a doneUnrecoverable ticket for a human to
// inspect: its commits never landed on featureBranch and no iteration branch
// survived to recover them from, so — unlike doneRecoverable — there's
// nothing here to auto-repair. It reuses the same needs-attention
// status/event machinery as Codex's own operator-intervention path rather
// than silently reverting the ticket to open (which would re-run it from
// scratch without a human ever knowing the first run's result vanished).
func markDoneTicketUnrecoverable(paths reconcilePaths, featureBranch string, t tickets.Ticket) error {
	reason := fmt.Sprintf("done but commits missing from %s and iteration branch %s no longer exists to recover them", featureBranch, iterBranch(featureBranch, t.Identifier))
	if err := MarkNeedsAttentionWithReason(t.Path, reason); err != nil {
		return fmt.Errorf("marking ticket needs-attention: %w", err)
	}
	if err := logEvent(paths.ScratchDir, featureBranch, Event{Type: eventNeedsAttention, Ticket: t.Identifier, Reason: reason}); err != nil {
		return fmt.Errorf("logging needs-attention event: %w", err)
	}
	return nil
}
