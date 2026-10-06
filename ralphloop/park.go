package ralphloop

import (
	"fmt"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets/schema"
)

// parkRequest describes one park outcome. Kind is mandatory: every park event
// carries the failure fields (kind, address, reason) so a catalog can match it.
type parkRequest struct {
	ScratchDir string
	EpicName   string
	Ticket     string // identifier
	Path       string // ticket file
	Type       events.Type
	Kind       events.Kind
	Reason     string
	// Repair is rendered into the ticket's "## Needs Repair" section.
	Repair schema.NeedsRepairState
}

// park is the single park path: it writes the ticket, appends exactly one
// event and notifies. Only needs-repair is routed here so far; the other
// status writers move over in later tickets.
//
// A failed ticket write is folded into the returned reason rather than
// aborting, so the event and notification still fire — a silent park is the
// bug this exists to remove.
func park(sink EventSink, req parkRequest) (reason string) {
	reason = req.Reason
	if req.Type != events.NeedsRepair {
		reason = fmt.Sprintf("%s (park: unsupported type %q)", reason, req.Type)
	} else if err := MarkNeedsRepairWithReason(req.Path, reason, schema.ParkKind(req.Kind), req.Repair); err != nil {
		reason = fmt.Sprintf("%s (also failed marking needs-repair: %v)", reason, err)
	}
	if err := logEvent(req.ScratchDir, req.EpicName, Event{
		Type:   string(req.Type),
		Ticket: req.Ticket,
		Kind:   string(req.Kind),
		Reason: reason,
	}); err != nil {
		reason = fmt.Sprintf("%s (also failed logging event: %v)", reason, err)
	}
	sink.TicketNeedsHuman(req.Ticket, req.EpicName, string(req.Type), reason)
	return reason
}
