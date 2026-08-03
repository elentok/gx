package ralphloop

import (
	"fmt"

	"github.com/elentok/gx/tickets"
)

// doneMismatchClass is how a done ticket's recorded landed commit compares
// against the feature branch's current tip, plus whether its iteration state
// was left behind — see classifyDoneTicket.
type doneMismatchClass int

const (
	doneOK doneMismatchClass = iota
	doneStaleCleanup
	doneRecoverable
	doneUnrecoverable
)

// classifyDoneTicket checks a done ticket's iteration commits against the
// feature branch's current tip and for leftover iteration state (tab,
// worktree, or branch never cleaned up), producing one of four
// classifications:
//
//   - doneOK: the commit landed and nothing was left behind.
//   - doneStaleCleanup: the commit landed, but a tab/worktree/branch is
//     still around — the crash landed between marking done and the cleanup
//     step right after it.
//   - doneRecoverable: the commit is missing, but the iteration branch still
//     holds it.
//   - doneUnrecoverable: the commit is missing and no iteration branch is
//     left to recover it from.
//
// Presence is checked first against the SHA recorded on the ticket's most
// recent cherry-picked event, not the iteration branch's own commits:
// CherryPickRange creates fresh commits on the feature branch (different
// hashes than the iteration branch's originals), so the iteration branch's
// tip is never literally reachable from the feature branch even in the
// fully-landed case. A done ticket with no recorded event (e.g. a run-log
// predating this check) starts out treated the same as a missing commit.
// Either way, if the iteration branch still exists, presence gets a second
// look via PatchesApplied (patch-id, not hash) before concluding the commit
// is really missing — the recorded SHA can also go stale harmlessly, e.g.
// when the feature branch is rebased after landing it, and blindly trusting
// that would misclassify already-landed work as doneRecoverable and
// re-cherry-pick it onto the live feature worktree. A third and final check,
// TrailerCommitExists, runs regardless of whether the iteration branch
// survived: it looks for the ticket-identifying trailer landCherryPick
// stamps on every landed commit, the only signal that survives a rebase
// where the commit was also manually re-resolved during a conflict — that
// changes both its hash and its patch-id, but never its commit message.
func classifyDoneTicket(d Deps, paths reconcilePaths, featureBranch string, t tickets.Ticket, events []Event, live map[string]bool) (doneMismatchClass, error) {
	landedSHA := latestCherryPickedSHA(events, t.Identifier)

	commitsPresent := false
	if landedSHA != "" {
		present, err := d.IsAncestor(paths.FeatureWorktree, landedSHA, featureBranch)
		if err != nil {
			return doneOK, fmt.Errorf("checking landed commit reachability: %w", err)
		}
		commitsPresent = present
	}

	branch := iterBranch(featureBranch, t.Identifier)
	hasBranch := branchExists(d, paths.FeatureWorktree, branch)

	// A missing landedSHA doesn't only mean the commit never landed — it also
	// happens harmlessly whenever featureBranch was rebased/amended after
	// landing it (rewriting hashes) or the recording event itself was lost.
	// Before treating that as a real gap, check whether the iteration
	// branch's content already made it onto featureBranch under different
	// hashes: falsely calling this doneRecoverable would re-cherry-pick
	// already-landed commits straight onto the live feature worktree.
	if !commitsPresent && hasBranch {
		base, err := d.MergeBase(paths.FeatureWorktree, branch, featureBranch)
		if err != nil {
			return doneOK, fmt.Errorf("resolving merge-base for patch-equivalence check: %w", err)
		}
		applied, err := d.PatchesApplied(paths.FeatureWorktree, featureBranch, base, branch)
		if err != nil {
			return doneOK, fmt.Errorf("checking patch-equivalent landed commits: %w", err)
		}
		commitsPresent = applied
	}

	// Last resort: neither SHA reachability nor patch-id equivalence can
	// survive a rebase where the landed commit was also manually re-resolved
	// during a conflict, since that changes both its hash and its diff. The
	// trailer landCherryPick stamps on every landed commit (Deps.AppendTrailer)
	// has neither problem — commit messages ride along through a rebase
	// untouched — so it's checked directly against the ticket, independent of
	// whether the iteration branch itself survived.
	if !commitsPresent {
		found, err := d.TrailerCommitExists(paths.FeatureWorktree, featureBranch, ticketTrailerKey, ticketTrailerValue(featureBranch, t.Identifier))
		if err != nil {
			return doneOK, fmt.Errorf("checking ticket trailer marker: %w", err)
		}
		commitsPresent = found
	}

	label := iterLabel(t.Identifier)
	hasWorktree, err := d.WorktreeExists(iterationWorktreePath(paths.WorktreeDir, featureBranch, t.Identifier))
	if err != nil {
		return doneOK, fmt.Errorf("checking leftover worktree: %w", err)
	}

	leftover := live[iterationKey(featureBranch, label)] || hasWorktree || hasBranch

	switch {
	case commitsPresent && !leftover:
		return doneOK, nil
	case commitsPresent:
		return doneStaleCleanup, nil
	case hasBranch:
		return doneRecoverable, nil
	default:
		return doneUnrecoverable, nil
	}
}

// latestCherryPickedSHA returns the SHA recorded on the most recent
// cherry-picked event logged for identifier (a ticket's Identifier, not
// Number, so lettered split siblings sharing a Number aren't
// cross-attributed), or "" if none was ever logged.
func latestCherryPickedSHA(events []Event, identifier string) string {
	sha := ""
	for _, ev := range events {
		if ev.Type == eventCherryPicked && ev.Ticket == identifier && ev.SHA != "" {
			sha = ev.SHA
		}
	}
	return sha
}
