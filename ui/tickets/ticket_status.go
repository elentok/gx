package tickets

import gxtickets "github.com/elentok/gx/tickets"

// ticketStatus is the status the tab shows for t: the server's when the ticket
// came from a snapshot, else computed from the epic, as when the tab reads the
// store itself.
func ticketStatus(epic gxtickets.Epic, t gxtickets.Ticket) gxtickets.RenderedStatus {
	if t.Rendered != nil {
		return *t.Rendered
	}
	return epic.RenderedStatus(t)
}
