package ralphloop

import (
	"fmt"
	"path/filepath"
	"strings"

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
func reconcile(d Deps, workspaceID string, paths reconcilePaths, epic tickets.Epic, report func(string, ...any)) ([]tickets.Ticket, error) {
	tabs, err := d.TabList(workspaceID)
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
			if live[iterLabel(t.Number)] {
				report("ticket %02d: reattaching to needs-attention iteration %s\n", t.Number, iterLabel(t.Number))
				reattached = append(reattached, t)
			} else {
				report("ticket %02d still needs attention; no live iteration found\n", t.Number)
			}
			continue
		}
		if status != "claimed" {
			continue
		}
		if !live[iterLabel(t.Number)] {
			if err := SetStatus(t.Path, "open"); err != nil {
				return nil, fmt.Errorf("reverting ticket %d to open: %w", t.Number, err)
			}
			report("ticket %02d: no live iteration found on restart; reverted to open\n", t.Number)
			continue
		}
		report("ticket %02d: reattaching to live iteration %s\n", t.Number, iterLabel(t.Number))
		reattached = append(reattached, t)
	}

	events, _, err := readEvents(paths.ScratchDir, epic.Name)
	if err != nil {
		return nil, fmt.Errorf("reading run log for done-ticket verification: %w", err)
	}
	for _, t := range epic.Tickets {
		if !t.IsDone() {
			continue
		}
		class, err := classifyDoneTicket(d, paths, epic.Name, t, events, live)
		if err != nil {
			return nil, fmt.Errorf("verifying done ticket %d: %w", t.Number, err)
		}
		switch class {
		case doneOK:
			// Commits landed, nothing left behind: the common case, left
			// untouched.
		case doneStaleCleanup:
			report("ticket %02d: done and commits landed, but leftover iteration state was never cleaned up\n", t.Number)
		case doneRecoverable:
			report("ticket %02d: done but commits missing from %s; iteration branch %s still has them\n", t.Number, epic.Name, iterBranch(t.Number))
		case doneUnrecoverable:
			report("ticket %02d: done but commits missing from %s and no iteration branch left to recover them\n", t.Number, epic.Name)
		}
	}

	return reattached, nil
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
// Presence is checked against the SHA recorded on the ticket's most recent
// cherry-picked event, not the iteration branch's own commits: CherryPickRange
// creates fresh commits on the feature branch (different hashes than the
// iteration branch's originals), so the iteration branch's tip is never
// literally reachable from the feature branch even in the fully-landed case.
// A done ticket with no recorded event (e.g. a run-log predating this check)
// is treated the same as a missing commit, since there's nothing to verify
// reachability against.
func classifyDoneTicket(d Deps, paths reconcilePaths, featureBranch string, t tickets.Ticket, events []Event, live map[string]bool) (doneMismatchClass, error) {
	landedSHA := latestCherryPickedSHA(events, t.Number)

	commitsPresent := false
	if landedSHA != "" {
		present, err := d.IsAncestor(paths.FeatureWorktree, landedSHA, featureBranch)
		if err != nil {
			return doneOK, fmt.Errorf("checking landed commit reachability: %w", err)
		}
		commitsPresent = present
	}

	branch := iterBranch(t.Number)
	_, revErr := d.RevParse(paths.FeatureWorktree, branch)
	branchExists := revErr == nil

	label := iterLabel(t.Number)
	hasWorktree, err := d.WorktreeExists(filepath.Join(paths.WorktreeDir, label))
	if err != nil {
		return doneOK, fmt.Errorf("checking leftover worktree: %w", err)
	}

	leftover := live[label] || hasWorktree || branchExists

	switch {
	case commitsPresent && !leftover:
		return doneOK, nil
	case commitsPresent:
		return doneStaleCleanup, nil
	case branchExists:
		return doneRecoverable, nil
	default:
		return doneUnrecoverable, nil
	}
}

// latestCherryPickedSHA returns the SHA recorded on the most recent
// cherry-picked event logged for ticketNumber, or "" if none was ever
// logged.
func latestCherryPickedSHA(events []Event, ticketNumber int) string {
	sha := ""
	for _, ev := range events {
		if ev.Type == eventCherryPicked && ev.Ticket == ticketNumber && ev.SHA != "" {
			sha = ev.SHA
		}
	}
	return sha
}
