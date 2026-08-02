package ralphloop

import (
	"fmt"
	"path/filepath"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
)

// reconcileOrphanedClaim handles a ticket left `Status: claimed` whose iter-NN
// tab isn't live: rather than reverting straight to open — which would
// abandon the iteration branch in place, only to collide with (and force a
// blind, possibly data-losing manual deletion of) the same branch/worktree
// name the next fresh attempt tries to create — it first checks whether that
// branch already holds commits the previous invocation never cherry-picked
// (e.g. the agent finished a smart-zone-paused iteration while no operator/
// process was alive to relay the resume signal). If so, it lands them through
// the same cherry-pick-and-cleanup path a normal completion uses, so the
// worktree/branch are only ever removed right after their commits are safely
// on the feature branch. Only when there's nothing to lose — no iteration
// branch, or one with zero commits ahead — does it fall back to the plain
// revert-to-open.
func reconcileOrphanedClaim(d Deps, rp reconcileParams, featureBranch string, t tickets.Ticket, tabs []herdr.Tab) error {
	paths := rp.Paths
	branch := iterBranch(featureBranch, t.Identifier)
	label := iterLabel(t.Identifier)
	path := filepath.Join(paths.WorktreeDir, label)

	if branchExists(d, paths.FeatureWorktree, branch) {
		base, err := d.MergeBase(paths.FeatureWorktree, branch, featureBranch)
		if err != nil {
			return fmt.Errorf("resolving %s's base for orphaned-claim recovery check: %w", branch, err)
		}
		ahead, err := d.CommitsAhead(paths.FeatureWorktree, base, branch)
		if err != nil {
			return fmt.Errorf("counting %s's commits ahead of %s: %w", branch, base, err)
		}
		if ahead > 0 {
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
				SmartZone:        rp.SmartZone,
				Gate:             rp.Gate,
				ResumeSignalPath: rp.ResumeSignalPath,
				Sink:             rp.Sink,
			}

			// Seed a live row so landCherryPick's CherryPickStarted/
			// ConflictResolutionStarted events below have a live entry to
			// update — without this, the tickets tab shows nothing while
			// this recovery cherry-pick runs.
			rp.Sink.TicketRecovering(t.Identifier)

			landedSHA, err := landCherryPick(d, p, base, branch, "", "", "")
			if err != nil {
				return fmt.Errorf("re-cherry-picking orphaned claim %s: %w", t.Identifier, err)
			}
			p.logTicketEventSHA(eventCherryPicked, "", "", "", path, "", landedSHA)

			if err := markDoneStampingCloseMetadata(d, p, path, ""); err != nil {
				return fmt.Errorf("marking recovered ticket %s done: %w", t.Identifier, err)
			}

			tabID := tabIDForLabel(tabs, label)
			if err := finishCleanup(d, paths.RepoDir, paths.FeatureWorktree, path, branch, tabID); err != nil {
				return fmt.Errorf("cleaning up orphaned claim %s: %w", t.Identifier, err)
			}

			rp.Sink.TicketRecovered(t.Identifier, featureBranch, branch, landedSHA)
			return nil
		}
	}

	if err := SetStatus(t.Path, "open"); err != nil {
		return fmt.Errorf("reverting ticket %s to open: %w", t.Identifier, err)
	}
	rp.Sink.TicketReverted(t.Identifier)
	return nil
}
