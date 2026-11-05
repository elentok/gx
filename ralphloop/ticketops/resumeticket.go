package ticketops

import (
	"fmt"
	"time"

	"github.com/elentok/gx/tickets/schema"
)

// ResumeTicket reopens the paused (answer) ticket at path and demotes its
// "## Needs Answer" stub into a dated "## Comments" entry — the same write a
// person makes by hand-editing status: open. Exported so a person can trigger
// it explicitly (e.g. the tickets/queue tabs' "m" suggested-actions menu).
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
		*body = schema.DemotePauseSections(*body, now)
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
		if t.PausedForRepair() {
			t.Status = schema.StatusOpen
			t.PauseReason = ""
		}
		*body = schema.DemotePauseSections(*body, now)
		*body = schema.AppendComment(*body, fmt.Sprintf("**%s** — unmuted", now.Format("2006-01-02")))
	})
}
