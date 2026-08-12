package ralphloop

import (
	"fmt"
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
		agent, live := liveAgent(d, workspaceID, epicName, agentKind, worktreeDir, t)
		if !live || agent.AgentStatus == "blocked" {
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
		if _, live := liveAgent(d, workspaceID, epicName, agentKind, worktreeDir, t); live {
			return true
		}
	}
	return false
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
