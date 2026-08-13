package ralphloop

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// unparkAnswered walks epic's in-scope needs-answer tickets and, for each
// whose iteration pane is still live and has left the blocked state, retires
// its "## Needs Answer" stub into "## Comments" and reopens it. The ticket
// then travels the ordinary reattach path claimNext already applies to any
// ticket cleared back to open (see resumeReattachable) — this pre-pass is
// what makes answering in the pane alone the whole unpark gesture, with no
// ownership check or generation counter: a person hand-editing the same
// ticket to open hits the identical path, so there is nothing here for a
// manual unpark to race.
//
// The live-pane check is what tells the two park kinds apart. A gate park
// (waitForFinish's parkOnBlockedPane) still owns its pane, so answering it
// leaves it live and unblocked — auto-unparked here. An announce-and-stop
// park has already released its pane/tab/worktree, so the check reports not
// live and this pass skips it, leaving it to be answered in the file by a
// person. Nothing records which kind a park was; this predicate answers it
// fresh every pass, including for a gate park whose pane a person killed —
// that degrades into a file-answered park rather than getting stuck.
func unparkAnswered(d Deps, workspaceID, epicName, worktreeDir string, agentKind AgentKind, scope RunScope, epic tickets.Epic, now time.Time) error {
	for _, t := range epic.Tickets {
		if !scope.Contains(t, epic) {
			continue
		}
		if epic.RenderedStatus(t) != tickets.StatusNeedsAnswer {
			continue
		}
		if !clearableParkedTicket(d, workspaceID, epicName, worktreeDir, agentKind, epic, t) {
			continue
		}
		if err := UnparkTicket(t.Path, now); err != nil {
			return fmt.Errorf("unparking answered ticket %s: %w", t.Identifier, err)
		}
	}
	return nil
}

// hasLiveParkedTicket reports whether epic has any in-scope needs-answer
// ticket whose iteration pane is still live — i.e. one unparkAnswered could
// plausibly clear on some future pass, once its pane leaves the blocked
// state. This is deliberately narrower than "something is parked": a
// needs-answer ticket whose pane is already gone (an announce-and-stop park)
// can only ever be cleared by a person editing the file, which the ordinary
// per-iteration epic reload already picks up — polling for it while a
// sibling runs would just race an always-ready park-timer fake against that
// sibling's results send and starve it out.
func hasLiveParkedTicket(d Deps, workspaceID, epicName, worktreeDir string, agentKind AgentKind, scope RunScope, epic tickets.Epic) bool {
	for _, t := range epic.Tickets {
		if !scope.Contains(t, epic) {
			continue
		}
		if epic.RenderedStatus(t) != tickets.StatusNeedsAnswer {
			continue
		}
		agent, live := liveAgent(d, workspaceID, epicName, agentKind, worktreeDir, t)
		if !live {
			continue
		}
		// A still-blocked pane could leave the blocked state on its own before
		// the next poll — the only way that gets noticed is this same poll
		// loop reaching unparkAnswered again — so it's worth continuing to
		// poll for regardless of what clearableParkedTicket says right now.
		// A live-and-unblocked ticket clearableParkedTicket still calls not
		// clearable (a zero-commit park with nothing landed yet) has no
		// pending state change a poll could catch: it only clears once a
		// person edits the file, which the ordinary per-iteration epic reload
		// already picks up, same as an announce-and-stop park's dead pane.
		if agent.AgentStatus == "blocked" || clearableParkedTicket(d, workspaceID, epicName, worktreeDir, agentKind, epic, t) {
			return true
		}
	}
	return false
}

// clearableParkedTicket reports whether t, parked at epic.RenderedStatus(t),
// is ready for gx to clear on its own — reopening a needs-answer ticket via
// unparkAnswered, or counting it "reattachable" in an EpicParked
// notification. Branches on RenderedStatus rather than t's raw Status: the
// three call sites reach status two different ways today, and a raw-Status
// predicate would route a ticket down the wrong branch whenever they
// diverge.
//
// A needs-answer ticket's clearability depends on which of ralph-loop's
// three needs-answer producers parked it (see schema.ParkKind's doc). A
// missing park_kind (any ticket parked before that field existed) defaults
// to ParkKindZeroCommit, the conservative choice: unlike a gate park or a
// self-report, a zero-commit finish never told anyone whether picking the
// pane back up is actually safe. blocked-pane and self-reported parks still
// own a pane an operator answers directly, so a live, unblocked pane is the
// whole signal, same as before park_kind existed. A zero-commit park's pane
// is left alive only for inspection — its agent already declared itself
// done with nothing to land — so liveness alone would auto-reattach a
// finished agent with no new instructions; CommitsAhead is what tells the
// zero-commit case apart, since it's the same check finishIteration itself
// uses to decide whether an iteration produced anything. The CommitsAhead
// call is gated behind park_kind == zero-commit && live so the common case
// (blocked-pane/self-reported) never pays for it, and a CommitsAhead
// failure degrades to "not clearable" rather than propagating as an error —
// a git hiccup here must not kill the whole run.
//
// needs-repair and draft parks never carry park_kind (only a needs-answer
// park does) and are unaffected by any of this: they stay the same
// liveness-only rule ralph-loop has always applied to them.
func clearableParkedTicket(d Deps, workspaceID, epicName, worktreeDir string, agentKind AgentKind, epic tickets.Epic, t tickets.Ticket) bool {
	switch epic.RenderedStatus(t) {
	case tickets.StatusNeedsAnswer:
		agent, live := liveAgent(d, workspaceID, epicName, agentKind, worktreeDir, t)
		if !live || agent.AgentStatus == "blocked" {
			return false
		}
		kind := t.ParkKind
		if kind == "" {
			kind = schema.ParkKindZeroCommit
		}
		if kind == schema.ParkKindZeroCommit {
			return parkedTicketHasNewCommits(d, worktreeDir, epicName, t)
		}
		return true
	case tickets.StatusNeedsRepair, tickets.StatusDraft:
		_, live := liveAgent(d, workspaceID, epicName, agentKind, worktreeDir, t)
		return live
	default:
		return false
	}
}

// parkedTicketHasNewCommits reports whether t's iteration branch holds any
// commits ahead of its original base — the same zero-commit test
// finishIteration itself runs, re-derived here since a zero-commit park
// records neither the base it was measured against nor the count itself.
// Any failure (the branch is gone, the merge-base can't resolve, git
// itself errors) degrades to false rather than propagating: a transient git
// hiccup must never turn into "not clearable" becoming a run-ending error.
func parkedTicketHasNewCommits(d Deps, worktreeDir, epicName string, t tickets.Ticket) bool {
	featureWorktree := filepath.Join(worktreeDir, epicName)
	branch := iterBranch(epicName, t.Identifier)
	if !branchExists(d, featureWorktree, branch) {
		return false
	}
	base, err := d.MergeBase(featureWorktree, branch, epicName)
	if err != nil {
		return false
	}
	ahead, err := d.CommitsAhead(featureWorktree, base, branch)
	if err != nil {
		return false
	}
	return ahead > 0
}

// UnparkTicket reopens the needs-answer ticket at path and demotes its
// "## Needs Answer" stub into a dated "## Comments" entry — the automatic
// counterpart of a person hand-editing status: open (see unparkAnswered).
// Exported so a person can trigger the same write explicitly (e.g. the
// tickets/queue tabs' "m" suggested-actions menu) for a park whose pane
// unparkAnswered can't safely auto-clear on its own (an announce-and-stop
// park's live-but-idle pane looks identical to an answered gate park).
func UnparkTicket(path string, now time.Time) error {
	return updateTicketWithBody(path, func(t *schema.Ticket, body *string) {
		t.Status = schema.StatusOpen
		*body = demoteSection(*body, "## Needs Answer", now)
	})
}
