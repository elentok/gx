package ralphloop

import (
	"fmt"
	"strings"
	"sync"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
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
	WorkspaceID string
	Paths       reconcilePaths
	Agent       AgentKind
	Skill       string
	SmartZone   int
	Gate        *Gate
	// WorktreeLock is the same Run-scoped lock as iterationParams.WorktreeLock
	// (see its doc), threaded through so startup repairs that add/remove a
	// worktree serialize against any concurrently-launched iteration's own
	// worktree add/remove.
	WorktreeLock *sync.Mutex
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
	liveTabs := make(map[string]herdr.Tab, len(tabs))
	for _, tab := range tabs {
		key := iterationKey(epic.Name, tab.Label)
		live[key] = true
		liveTabs[key] = tab
	}

	events, _, err := readEvents(paths.ScratchDir, epic.Name)
	if err != nil {
		return nil, fmt.Errorf("reading run log for done-ticket verification: %w", err)
	}

	reattach := func(t tickets.Ticket) {
		label := iterLabel(epic.Name, t.Identifier)
		cwd := iterationWorktreePath(paths.WorktreeDir, epic.Name, t.Identifier)
		tab := liveTabs[iterationKey(epic.Name, label)]
		agentState, agentErr := d.AgentGet(label)
		sessionID := ""
		if agentErr == nil && agentState.PaneID != "" && agentState.TabID == tab.TabID && agentState.WorkspaceID == rp.WorkspaceID {
			sessionID = agentState.AgentSession
			if rp.Agent == AgentCodex {
				verified, verifyErr := d.VerifyCodexSession(cwd, sessionID)
				if verifyErr != nil || !verified {
					sessionID = ""
				}
			}
		}
		sink.TicketReattached(t.Identifier, label, cwd, sessionID)
		if sessionID != "" {
			emitContextOccupancy(d, sink, rp.Agent, t.Identifier, cwd, sessionID)
		}
	}

	var reattached []tickets.Ticket
	for _, t := range epic.Tickets {
		if !rp.Scope.Contains(t, epic) {
			// Belongs to a different (or not yet requested) run — leave its
			// claim/needs-repair state exactly as found rather than
			// reattaching, reverting, or reporting on it here.
			continue
		}
		status := strings.ToLower(strings.TrimSpace(t.Status))
		if status == "needs-repair" {
			if live[iterationKey(epic.Name, iterLabel(epic.Name, t.Identifier))] {
				reattach(t)
				reattached = append(reattached, t)
			} else {
				sink.TicketNeedsHuman(t.Identifier, epic.Name, "needs-repair", "no live iteration found")
			}
			continue
		}
		if status != "claimed" {
			continue
		}
		if t.Type == string(schema.TypeConflictResolution) {
			// A conflict-resolution child ticket never has its own iterLabel
			// tab (see conflictLabel's doc) — it's keyed off the parent's
			// identifier instead, since resolveCherryPickConflict launches its
			// pane inline inside the parent's own landCherryPick call. Check
			// liveness there rather than falling through to the generic
			// iterLabel check below, which would always read "not live" for
			// this ticket type and send a still-running resolver's child
			// record through reconcileOrphanedClaim to be wrongly reverted to
			// open out from under it.
			if t.Parent != nil && live[iterationKey(epic.Name, conflictLabel(*t.Parent))] {
				continue
			}
			if err := reconcileOrphanedClaim(d, rp, epic.Name, t, tabs); err != nil {
				return nil, fmt.Errorf("reconciling orphaned claim %s: %w", t.Identifier, err)
			}
			continue
		}
		if !live[iterationKey(epic.Name, iterLabel(epic.Name, t.Identifier))] {
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
		if t.IsDone() && !t.Commitless {
			landed, _ = LandedTickets(paths.FeatureWorktree, epic.Name)
			break
		}
	}

	for _, t := range epic.Tickets {
		if !rp.Scope.Contains(t, epic) {
			// Belongs to a different (or not yet requested) run — same
			// out-of-scope skip as the claim/needs-repair loop above, so a
			// scoped run never rewrites a done ticket's status outside what it
			// was asked to touch.
			continue
		}
		// A commitless done ticket (see schema.Ticket.Commitless) never had a
		// commit to land in the first place — verifying it is skipped here.
		if !t.IsDone() || t.Commitless {
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
			reason, err := markDoneTicketUnrecoverable(paths, epic.Name, t)
			if err != nil {
				return nil, fmt.Errorf("flagging unrecoverable done ticket %s: %w", t.Identifier, err)
			}
			sink.TicketUnrecoverable(t.Identifier, epic.Name)
			sink.TicketNeedsHuman(t.Identifier, epic.Name, "needs-repair", reason)
		}
	}

	return reattached, nil
}
