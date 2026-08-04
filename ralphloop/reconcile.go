package ralphloop

import (
	"fmt"
	"strings"
	"sync"

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
	// Scope restricts which tickets reconcile reattaches or reports on — a
	// ticket outside it belongs to a different (or not yet started) run and
	// is left untouched, whatever its Status. Callers running the whole
	// epic pass ResolveRunScope's wholeEpic result, not the zero value (see
	// RunScope.Contains).
	Scope RunScope
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
		live[iterationKey(epic.Name, tab.Label)] = true
	}

	events, _, err := readEvents(paths.ScratchDir, epic.Name)
	if err != nil {
		return nil, fmt.Errorf("reading run log for done-ticket verification: %w", err)
	}

	reattach := func(t tickets.Ticket) {
		sessionID, cwd, agent, ok := lastIterationSession(events, t.Identifier)
		sink.TicketReattached(t.Identifier, iterLabel(t.Identifier), cwd, sessionID)
		if ok {
			emitContextOccupancy(d, sink, agent, t.Identifier, cwd, sessionID)
		}
	}

	var reattached []tickets.Ticket
	for _, t := range epic.Tickets {
		if !rp.Scope.Contains(t) {
			// Belongs to a different (or not yet requested) run — leave its
			// claim/needs-attention state exactly as found rather than
			// reattaching, reverting, or reporting on it here.
			continue
		}
		status := strings.ToLower(strings.TrimSpace(t.Status))
		if status == "needs-attention" {
			if live[iterationKey(epic.Name, iterLabel(t.Identifier))] {
				reattach(t)
				reattached = append(reattached, t)
			} else {
				sink.TicketStillNeedsAttention(t.Identifier)
			}
			continue
		}
		if status != "claimed" {
			continue
		}
		if !live[iterationKey(epic.Name, iterLabel(t.Identifier))] {
			if err := reconcileOrphanedClaim(d, rp, epic.Name, t, tabs); err != nil {
				return nil, fmt.Errorf("reconciling orphaned claim %s: %w", t.Identifier, err)
			}
			continue
		}
		reattach(t)
		reattached = append(reattached, t)
	}

	// Computed once per run (not per ticket) and threaded into every
	// classifyDoneTicket call below — a real perf win over the one
	// TrailerCommitExists shell-out per done ticket this replaced. Only
	// attempted when there's at least one done ticket to check, so most runs
	// (nothing done yet) skip the git call entirely. A failure here (e.g. a
	// transient git error) degrades to an empty map rather than aborting
	// reconcile: classifyDoneTicket's trailer-based fallback simply
	// contributes nothing that run, while IsAncestor/PatchesApplied still
	// resolve the common cases.
	var landed map[string]bool
	for _, t := range epic.Tickets {
		if t.IsDone() && !t.IsSuperseded() && !t.Commitless {
			landed, _ = LandedTickets(paths.FeatureWorktree, epic.Name)
			break
		}
	}

	for _, t := range epic.Tickets {
		// A commitless done ticket (see schema.Ticket.Commitless) never had a
		// commit to land in the first place — verifying it the same way as
		// t.IsSuperseded() is skipped here, for the same reason.
		if !t.IsDone() || t.IsSuperseded() || t.Commitless {
			continue
		}
		class, err := classifyDoneTicket(d, paths, epic.Name, t, events, live, landed)
		if err != nil {
			return nil, fmt.Errorf("verifying done ticket %s: %w", t.Identifier, err)
		}
		switch class {
		case doneOK:
			// Commits landed, nothing left behind: the common case, left
			// untouched.
		case doneStaleCleanup:
			if err := finishStaleCleanup(d, rp, epic.Name, t, tabs); err != nil {
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
