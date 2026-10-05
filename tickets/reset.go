package tickets

import (
	"time"

	"github.com/elentok/gx/tickets/schema"
)

// Reset returns the ticket at path to a fresh open state: every machine-written
// run field is zeroed and any "## Needs Repair" / "## Needs Answer" section is
// retired into "## Comments" verbatim. Human-authored frontmatter (id,
// blocked_by, parent, type, expected_context_window) and every other body
// section are left as written. A non-empty note is appended to "## Comments"
// in the same write, so the file never shows the reset without its note.
func Reset(path string, now time.Time, note string) error {
	return schema.UpdateTicketWithBody(path, func(t *schema.Ticket, body *string) {
		t.Status = schema.StatusOpen
		t.IterationStatus = ""
		t.ParkKind = ""
		t.SessionIDs = nil
		t.ActualContextWindow = 0
		t.ElapsedTime = 0
		t.ActualCost = 0
		t.Compactions = 0
		t.Commitless = false
		*body = schema.DemoteSection(*body, "## Needs Repair", now)
		*body = schema.DemoteSection(*body, "## Needs Answer", now)
		if note != "" {
			*body = schema.AppendComment(*body, note)
		}
	})
}
