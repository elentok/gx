package ralphloop

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// resumeAnswered walks epic's in-scope paused (answer) tickets and, for each
// whose iteration pane is still live and has left the blocked state, retires
// its "## Needs Answer" stub into "## Comments" and reopens it. The ticket
// then travels the ordinary reattach path claimNext already applies to any
// ticket cleared back to open (see resumeReattachable) — this pre-pass is
// what makes answering in the pane alone the whole resume gesture, with no
// ownership check or generation counter: a person hand-editing the same
// ticket to open hits the identical path, so there is nothing here for a
// manual resume to race.
//
// The live-pane check is what tells the two pause kinds apart. A gate pause
// (waitForFinish's pauseOnBlockedPane) still owns its pane, so answering it
// leaves it live and unblocked — auto-resumed here. An announce-and-stop
// pause has already released its pane/tab/worktree, so the check reports not
// live and this pass skips it, leaving it to be answered in the file by a
// person. Nothing records which kind a pause was; this predicate answers it
// fresh every pass, including for a gate pause whose pane a person killed —
// that degrades into a file-answered pause rather than getting stuck.
func resumeAnswered(d Deps, epicName, worktreeDir string, agentKind AgentKind, scope RunScope, epic tickets.Epic, now time.Time) error {
	for _, t := range epic.Tickets {
		if !scope.Contains(t, epic) {
			continue
		}
		if epic.RenderedStatus(t) != tickets.StatusPausedAnswer {
			continue
		}
		if !clearablePausedTicket(d, epicName, worktreeDir, agentKind, epic, t) {
			continue
		}
		if err := ResumeTicket(t.Path, now); err != nil {
			return fmt.Errorf("resuming answered ticket %s: %w", t.Identifier, err)
		}
	}
	return nil
}

// hasLivePausedTicket reports whether epic has any in-scope paused (answer)
// ticket whose iteration pane is still live — i.e. one resumeAnswered could
// plausibly clear on some future pass, once its pane leaves the blocked
// state. This is deliberately narrower than "something is paused": a
// paused (answer) ticket whose pane is already gone (an announce-and-stop pause)
// can only ever be cleared by a person editing the file, which the ordinary
// per-iteration epic reload already picks up — polling for it while a
// sibling runs would just race an always-ready pause-timer fake against that
// sibling's results send and starve it out.
func hasLivePausedTicket(d Deps, epicName, worktreeDir string, agentKind AgentKind, scope RunScope, epic tickets.Epic) bool {
	for _, t := range epic.Tickets {
		if !scope.Contains(t, epic) {
			continue
		}
		if epic.RenderedStatus(t) != tickets.StatusPausedAnswer {
			continue
		}
		status, live := liveAgent(d, epicName, agentKind, worktreeDir, t)
		if !live {
			continue
		}
		// A still-blocked pane could leave the blocked state on its own before
		// the next poll — the only way that gets noticed is this same poll
		// loop reaching resumeAnswered again — so it's worth continuing to
		// poll for regardless of what clearableAsking says right now.
		// A live-and-unblocked ticket clearableAsking still calls not
		// clearable (a zero-commit pause with nothing merged yet) has no
		// pending state change a poll could catch: it only clears once a
		// person edits the file, which the ordinary per-iteration epic reload
		// already picks up, same as an announce-and-stop pause's dead pane.
		//
		// agent/live were just looked up above for this same ticket — reused
		// here via clearableAsking rather than re-derived through
		// clearablePausedTicket, which would look the pane up a second time.
		if status.State == agentrunner.StateBlocked || clearableAsking(d, worktreeDir, epicName, t, status, live) {
			return true
		}
	}
	return false
}

// clearablePausedTicket reports whether t, paused at epic.RenderedStatus(t),
// is ready for gx to clear on its own — reopening a paused (answer) ticket via
// resumeAnswered, or counting it "reattachable" in an EpicPaused
// notification. Branches on RenderedStatus rather than t's raw Status: the
// three call sites reach status two different ways today, and a raw-Status
// predicate would route a ticket down the wrong branch whenever they
// diverge.
func clearablePausedTicket(d Deps, epicName, worktreeDir string, agentKind AgentKind, epic tickets.Epic, t tickets.Ticket) bool {
	switch epic.RenderedStatus(t) {
	case tickets.StatusPausedAnswer:
		status, live := liveAgent(d, epicName, agentKind, worktreeDir, t)
		return clearableAsking(d, worktreeDir, epicName, t, status, live)
	case tickets.StatusPausedRepair, tickets.StatusDraft:
		// These statuses never carry pause_kind (only a paused (answer) pause
		// does) and stay on the same liveness-only rule ralph-loop has
		// always applied to them.
		_, live := liveAgent(d, epicName, agentKind, worktreeDir, t)
		return live
	default:
		return false
	}
}

// clearableAsking is clearablePausedTicket's asking branch,
// split out so a caller that has already looked up t's live-agent status
// (hasLivePausedTicket, deciding whether to keep polling the same ticket) can
// reuse that lookup instead of paying for a second pane lookup here.
//
// Clearability depends on which of ralph-loop's three paused (answer) producers
// paused t (see schema.PauseKind's doc). A missing pause_kind (any ticket
// paused before that field existed) defaults to PauseKindZeroCommit, the
// conservative choice: unlike a gate pause or a self-report, a zero-commit
// finish never told anyone whether picking the pane back up is actually
// safe. blocked-pane and self-reported pauses still own a pane an operator
// answers directly, so a live, unblocked pane is the whole signal, same as
// before pause_kind existed. A zero-commit pause's pane is left alive only for
// inspection — its agent already declared itself done with nothing to merge —
// so liveness alone would auto-reattach a finished agent with no new
// instructions; CommitsAhead is what tells the zero-commit case apart, since
// it's the same check finishIteration itself uses to decide whether an
// iteration produced anything. The CommitsAhead call is gated behind
// pause_kind == zero-commit && live so the common case (blocked-pane/
// self-reported) never pays for it, and a CommitsAhead failure degrades to
// "not clearable" rather than propagating as an error — a git hiccup here
// must not kill the whole run.
func clearableAsking(d Deps, worktreeDir, epicName string, t tickets.Ticket, status agentrunner.Status, live bool) bool {
	if !live || status.State == agentrunner.StateBlocked {
		return false
	}
	kind := t.PauseKind
	if kind == "" {
		kind = schema.PauseKindZeroCommit
	}
	if kind == schema.PauseKindZeroCommit {
		return pausedTicketHasNewCommits(d, worktreeDir, epicName, t)
	}
	return true
}

// pausedTicketHasNewCommits reports whether t's iteration branch holds any
// commits ahead of its original base — the same zero-commit test
// finishIteration itself runs, re-derived here since a zero-commit pause
// records neither the base it was measured against nor the count itself.
// Any failure (the branch is gone, the merge-base can't resolve, git
// itself errors) degrades to false rather than propagating: a transient git
// hiccup must never turn into "not clearable" becoming a run-ending error.
func pausedTicketHasNewCommits(d Deps, worktreeDir, epicName string, t tickets.Ticket) bool {
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

// ResumeTicket reopens the paused (answer) ticket at path and demotes its
// "## Needs Answer" stub into a dated "## Comments" entry — the automatic
// counterpart of a person hand-editing status: open (see resumeAnswered).
// Exported so a person can trigger the same write explicitly (e.g. the
// tickets/queue tabs' "m" suggested-actions menu) for a pause whose pane
// resumeAnswered can't safely auto-clear on its own (an announce-and-stop
// pause's live-but-idle pane looks identical to an answered gate pause).
func ResumeTicket(path string, now time.Time) error {
	return ResumeTicketWithBase(path, now, "")
}

// ResumeTicketWithBase is ResumeTicket that also writes base: onto the ticket,
// the person's pick for an ambiguous-base pause; "" leaves it alone.
func ResumeTicketWithBase(path string, now time.Time, base string) error {
	return rewriteTicket(path, false, resumeMutation(now, base))
}

// ResumeTicketWithBaseLocked is ResumeTicketWithBase for a caller that already
// holds the ticket lock, so the resume can share one lock with a log append.
func ResumeTicketWithBaseLocked(path string, now time.Time, base string) error {
	return rewriteTicketLocked(path, false, resumeMutation(now, base))
}

func resumeMutation(now time.Time, base string) func(*schema.Ticket, *string) error {
	return func(t *schema.Ticket, body *string) error {
		t.Status = schema.StatusOpen
		if base != "" {
			t.Base = base
		}
		*body = schema.DemoteSection(*body, "## Needs Answer", now)
		return nil
	}
}

// UnmuteTicket clears the ticket at path's Mutes field and demotes any "##
// Needs Repair" section into a dated "## Comments" sub-entry, the storm-mute
// counterpart of ResumeTicket — triggered by a person via the tickets/queue
// tabs' "m" suggested-actions menu or the server's unmute route, since a storm mute (unlike a paused (answer)
// pause) never auto-clears on its own. If the mute actually paused the ticket
// (status: paused (repair), written when muteSource's pauseTicket callback ran),
// this also reopens it; a mute that tripped with no pauseTicket callback
// available (see muteSource) never changed status, so there's no pause to
// undo and clearing Mutes is the whole action.
func UnmuteTicket(path string, now time.Time) error {
	return updateTicketWithBody(path, func(t *schema.Ticket, body *string) {
		t.Mutes = nil
		if t.PausedClass() == schema.PauseClassRepair {
			t.Status = schema.StatusOpen
			t.PauseReason = ""
		}
		*body = schema.DemoteSection(*body, "## Needs Repair", now)
		*body = schema.AppendComment(*body, fmt.Sprintf("**%s** — unmuted", now.Format("2006-01-02")))
	})
}
