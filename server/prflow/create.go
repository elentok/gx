package prflow

import (
	"fmt"
	"net/http"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// CreateHost is what the Create PR route needs beyond Open.
type CreateHost interface {
	OpenHost
	// Epic is the loaded epic, false when the project or epic is unknown.
	Epic(project, epic string) (tickets.Epic, bool)
	// EpicState is the state the Epics page would show for the epic now.
	EpicState(root queue.RootRef, e tickets.Epic) wire.EpicState
}

// Routes lists the Create PR write with its web variant. The getter is called
// per request, so the route can be listed before the server has its handler.
func Routes(create func() func(wire.QueueRequest) (wire.QueueResult, error)) []wire.Route {
	return []wire.Route{{
		Pattern:    "POST /v1/epics/create-pr",
		Web:        wire.WebWrite,
		Handler:    func(w http.ResponseWriter, r *http.Request) { queue.Handler(create())(w, r) },
		WebHandler: func(w http.ResponseWriter, r *http.Request) { queue.HandlerWeb(create())(w, r) },
	}}
}

// Create is the manual PR trigger for an epic address: it runs Open once. A gh
// or push failure comes back as the stored reason, never as a pause.
func Create(h CreateHost, req wire.QueueRequest) (wire.QueueResult, error) {
	project, epic, ok := tickets.ParseEpicAddress(req.Address)
	if !ok {
		return queue.Refusal(wire.ReasonInvalidAddress, req.Address+" is not an epic address (want project:epic)"), nil
	}
	root := queue.RootRef{Project: project, Epic: epic}
	e, ok := h.Epic(project, epic)
	if !ok || len(e.Tickets) == 0 {
		return queue.Refusal(wire.ReasonUnknownTicket, "no epic "+root.String()), nil
	}
	switch {
	case e.IsMap:
		return queue.Refusal(wire.ReasonMapEpic, root.String()+" is a map epic; its tickets are resolved by hand"), nil
	case !root.Mergeable():
		return queue.Refusal(wire.ReasonOneOffsEpic, root.String()+" is the one-offs inbox; it never opens a PR"), nil
	case !e.MergedAt.IsZero():
		return queue.Refusal(wire.ReasonAlreadyMerged, root.String()+" is already merged"), nil
	case e.PRNumber != 0:
		return queue.Refusal(wire.ReasonHasPR, fmt.Sprintf("%s already has PR #%d", root, e.PRNumber)), nil
	case !complete(e):
		return queue.Refusal(wire.ReasonNotComplete, root.String()+" is not complete"), nil
	}
	if shown := wire.EpicState(req.State); shown != "" {
		if now := h.EpicState(root, e); shown != now {
			return queue.Refusal(wire.ReasonStale, fmt.Sprintf("%s is %s now, the page showed %s", root, now, shown)), nil
		}
	}
	out, err := Open(h, root)
	if err != nil {
		return wire.QueueResult{}, err
	}
	if out == OutcomeBlocked {
		return queue.Refusal(wire.ReasonPROpenFailed, h.Reasons().Reason(root)), nil
	}
	if out == OutcomeSkipped {
		return queue.Refusal(wire.ReasonNotComplete, root.String()+" is not ready for a PR"), nil
	}
	return wire.QueueResult{State: string(out)}, nil
}

// complete is openGate's ticket rule: every ticket but pr-text ones is done or
// cancelled, and at least one is.
func complete(e tickets.Epic) bool {
	real := 0
	for _, t := range e.Tickets {
		if t.Type == string(schema.TypePRText) {
			continue
		}
		if t.Status != string(schema.StatusDone) && t.Status != string(schema.StatusCancelled) {
			return false
		}
		real++
	}
	return real > 0
}
