package ticketops

import (
	"fmt"
	"time"

	"github.com/elentok/gx/ralphloop/ticketops"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// Unmute clears a ticket's mutes and reopens it if the mute paused it.
func (o *Ops) Unmute(req wire.QueueRequest) (wire.QueueResult, error) {
	return o.d.Writer.TicketWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		if len(ref.Ticket.Mutes) == 0 {
			return queue.Refusal(wire.ReasonNotMuted, ref.Addr.String()+" is not muted"), nil
		}
		if err := ticketops.UnmuteTicket(ref.Ticket.Path, time.Now()); err != nil {
			return wire.QueueResult{}, fmt.Errorf("unmute %s: %w", ref.Addr, err)
		}
		if err := tickets.EndOpenRound(ref.Ticket.Path, req.Source, tickets.HowResume, ticketops.CloseResumed); err != nil {
			return wire.QueueResult{}, fmt.Errorf("end pause round of %s: %w", ref.Addr, err)
		}
		o.d.Index.Rescan()
		return wire.QueueResult{}, nil
	})
}
