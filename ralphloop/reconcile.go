package ralphloop

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
)

// reconcilePaths bundles the run-scoped filesystem locations reconcile needs
// alongside the epic itself — the scratch dir (to read run-log.jsonl), the
// feature worktree (to run git checks in), and the iteration worktree
// directory (to build iter-NN paths for leftover checks). These three always
// travel together across a single Run call, so callers build one instead of
// threading three loose strings through reconcile and classifyDoneTicket.
type reconcilePaths struct {
	ScratchDir      string
	FeatureWorktree string
	WorktreeDir     string
	// RepoDir is only needed to repair a doneRecoverable ticket (removing its
	// leftover worktree), so it's left unset in tests that never reach that
	// path.
	RepoDir string
}

// reconcileParams bundles reconcile's fixed-for-the-run inputs: the
// workspace/paths every classification needs, plus everything a startup
// repair of a doneRecoverable ticket needs to drive the same
// iterationParams-shaped cherry-pick-with-conflict-resolution path a fresh
// iteration uses (see repairRecoverableTicket).
type reconcileParams struct {
	WorkspaceID      string
	Paths            reconcilePaths
	Agent            AgentKind
	Skill            string
	SmartZone        int
	Gate             *Gate
	ResumeSignalPath string
	FeatureLock      *sync.Mutex
	// Sink receives this Run call's lifecycle events, safe to call
	// concurrently.
	Sink EventSink
}

// reconcile derives in-flight iteration state from ticket Status: plus live
// herdr state, with no separate state file: it runs unconditionally at the
// start of every Run call. Every ticket left `Status: claimed` (from a prior
// crash, killed terminal, or any other exit) either still has a live iter-NN
// tab in the epic's workspace — returned here so the caller reattaches and
// resumes it — or doesn't, meaning nothing survived and the ticket is
// reverted to open so normal scheduling picks it up fresh. It also verifies
// every already-done ticket's landed commit is still reachable from the
// feature branch's current tip (see classifyDoneTicket) — status alone can
// silently drift from what's actually landed, since it lives in a file
// independent of which worktree touches it. Mismatches are only reported
// here, not repaired: that's later tickets' job.
func reconcile(d Deps, rp reconcileParams, epic tickets.Epic) ([]tickets.Ticket, error) {
	paths := rp.Paths
	sink := rp.Sink
	tabs, err := d.TabList(rp.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("listing tabs for crash/restart reconciliation: %w", err)
	}
	live := make(map[string]bool, len(tabs))
	for _, tab := range tabs {
		live[tab.Label] = true
	}

	var reattached []tickets.Ticket
	for _, t := range epic.Tickets {
		status := strings.ToLower(strings.TrimSpace(t.Status))
		if status == "needs-attention" {
			if live[iterLabel(t.Identifier)] {
				sink.TicketReattached(t.Identifier, iterLabel(t.Identifier))
				reattached = append(reattached, t)
			} else {
				sink.TicketStillNeedsAttention(t.Identifier)
			}
			continue
		}
		if status != "claimed" {
			continue
		}
		if !live[iterLabel(t.Identifier)] {
			if err := reconcileOrphanedClaim(d, rp, epic.Name, t, tabs); err != nil {
				return nil, fmt.Errorf("reconciling orphaned claim %s: %w", t.Identifier, err)
			}
			continue
		}
		sink.TicketReattached(t.Identifier, iterLabel(t.Identifier))
		reattached = append(reattached, t)
	}

	events, _, err := readEvents(paths.ScratchDir, epic.Name)
	if err != nil {
		return nil, fmt.Errorf("reading run log for done-ticket verification: %w", err)
	}
	for _, t := range epic.Tickets {
		if !t.IsDone() || t.IsSuperseded() {
			continue
		}
		class, err := classifyDoneTicket(d, paths, epic.Name, t, events, live)
		if err != nil {
			return nil, fmt.Errorf("verifying done ticket %s: %w", t.Identifier, err)
		}
		switch class {
		case doneOK:
			// Commits landed, nothing left behind: the common case, left
			// untouched.
		case doneStaleCleanup:
			if err := finishStaleCleanup(d, rp, t, tabs); err != nil {
				return nil, fmt.Errorf("finishing interrupted cleanup for done ticket %s: %w", t.Identifier, err)
			}
			sink.TicketCleanupFinished(t.Identifier)
		case doneRecoverable:
			if err := repairRecoverableTicket(d, rp, epic.Name, t, tabs); err != nil {
				return nil, fmt.Errorf("repairing done ticket %s: %w", t.Identifier, err)
			}
		case doneUnrecoverable:
			if err := markDoneTicketUnrecoverable(paths, epic.Name, t); err != nil {
				return nil, fmt.Errorf("flagging unrecoverable done ticket %s: %w", t.Identifier, err)
			}
			sink.TicketUnrecoverable(t.Identifier, epic.Name)
		}
	}

	return reattached, nil
}

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
	branch := iterBranch(t.Identifier)
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
	branch := iterBranch(t.Identifier)
	label := iterLabel(t.Identifier)
	path := filepath.Join(paths.WorktreeDir, label)

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
		SmartZone:        rp.SmartZone,
		Gate:             rp.Gate,
		ResumeSignalPath: rp.ResumeSignalPath,
		Sink:             rp.Sink,
	}

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
		if err := d.RemoveWorktree(paths.RepoDir, path, true); err != nil {
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
func finishStaleCleanup(d Deps, rp reconcileParams, t tickets.Ticket, tabs []herdr.Tab) error {
	paths := rp.Paths
	label := iterLabel(t.Identifier)
	branch := iterBranch(t.Identifier)
	path := filepath.Join(paths.WorktreeDir, label)
	tabID := tabIDForLabel(tabs, label)

	return finishCleanup(d, paths.RepoDir, paths.FeatureWorktree, path, branch, tabID)
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
	reason := fmt.Sprintf("done but commits missing from %s and iteration branch %s no longer exists to recover them", featureBranch, iterBranch(t.Identifier))
	if err := MarkNeedsAttention(t.Path); err != nil {
		return fmt.Errorf("marking ticket needs-attention: %w", err)
	}
	if err := logEvent(paths.ScratchDir, featureBranch, Event{Type: eventNeedsAttention, Ticket: t.Identifier, Reason: reason}); err != nil {
		return fmt.Errorf("logging needs-attention event: %w", err)
	}
	return nil
}

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

	branch := iterBranch(t.Identifier)
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
		found, err := d.TrailerCommitExists(paths.FeatureWorktree, featureBranch, ticketTrailerKey, t.Identifier)
		if err != nil {
			return doneOK, fmt.Errorf("checking ticket trailer marker: %w", err)
		}
		commitsPresent = found
	}

	label := iterLabel(t.Identifier)
	hasWorktree, err := d.WorktreeExists(filepath.Join(paths.WorktreeDir, label))
	if err != nil {
		return doneOK, fmt.Errorf("checking leftover worktree: %w", err)
	}

	leftover := live[label] || hasWorktree || hasBranch

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
