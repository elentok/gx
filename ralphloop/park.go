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
	// Event carries optional agent context (pane, tab, session, cwd) merged
	// into the appended event; park overwrites its type, ticket, kind and reason.
	Event Event
	// AlreadyDone skips the ticket write of a manual-land whose ticket was
	// already done.
	AlreadyDone bool
}

// NoopEventSink is the sink for park callers with no live TUI to notify
// (the CLI, the Queue tab's budget kill, which sends its own notification).
func NoopEventSink() EventSink { return noopEventSink{} }

// RecordManualLand writes done through the park path and appends the one
// manual-land event. Only the ticket write's error is returned, matching park.
func RecordManualLand(scratchDir, epicName, ticket, path string, alreadyDone bool, ev Event) error {
	_, err := park(noopEventSink{}, parkRequest{
		ScratchDir: scratchDir, EpicName: epicName, Ticket: ticket, Path: path,
		Type: events.ManualLand, Reason: ev.Reason, AlreadyDone: alreadyDone, Event: ev,
	})
	return err
}

// park is the single park path: it writes the ticket, appends exactly one
// event and notifies. Every status writer is routed here:
// needs-repair, needs-answer and the CLI land's done write.
//
// A failed ticket write is folded into the notified/logged reason so the event
// and notification still fire — a silent park is the bug this exists to
// remove — and is also returned, for callers whose flow must not continue past
// an unwritten park.
func park(sink EventSink, req parkRequest) (reason string, writeErr error) {
	reason = req.Reason
	switch req.Type {
	case events.NeedsRepair:
		writeErr = markNeedsRepairWithReason(req.Path, reason, schema.ParkKind(req.Kind), req.Repair)
	case events.NeedsAnswer:
		// A blocked pane's question lives only in the pane, so its ticket gets a
		// stub; ticket-answered parks (zero-commit, self-reported) stay bare.
		if req.Kind == events.BlockedPane {
			writeErr = markNeedsAnswerWithReasonAndStub(req.Path, reason, schema.ParkKind(req.Kind))
		} else {
			writeErr = markNeedsAnswer(req.Path, schema.ParkKind(req.Kind))
		}
	case events.ManualLand:
		if !req.AlreadyDone {
			writeErr = MarkDone(req.Path)
		}
	default:
		writeErr = fmt.Errorf("park: unsupported type %q", req.Type)
	}
	if writeErr != nil {
		reason = fmt.Sprintf("%s (also failed marking %s: %v)", reason, req.Type, writeErr)
	}
	ev := req.Event
	ev.Type, ev.Ticket, ev.Kind, ev.Reason = string(req.Type), req.Ticket, string(req.Kind), reason
	if err := logEvent(req.ScratchDir, req.EpicName, ev); err != nil {
		reason = fmt.Sprintf("%s (also failed logging event: %v)", reason, err)
	}
	if req.Type == events.ManualLand {
		return reason, writeErr // a land needs no human
	}
	sink.TicketNeedsHuman(req.Ticket, req.EpicName, string(req.Type), reason)
	return reason, writeErr
}
