package tickets

import gxtickets "github.com/elentok/gx/tickets"

// ticketStatus is the status the tab shows for t: the server's when the ticket
// came from a snapshot, else the file's own status. The blocked and
// waiting-for-children overlays are the server's to compute, so a tab reading
// the store itself shows none.
func ticketStatus(_ gxtickets.Epic, t gxtickets.Ticket) gxtickets.RenderedStatus {
	if t.Rendered != nil {
		return *t.Rendered
	}
	return t.RawStatus()
}
