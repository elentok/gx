package prflow

import (
	"fmt"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// TryAgain ends a "CI failing" or "PR text failed" pause: it logs a reset (the
// counts restart from it), clears the stored pause and reruns the failed step.
// A still-failing check gets a fresh ticket on the next poll; for a failed PR
// text a new pr-text ticket starts, and the PR opens once its text is valid.
func TryAgain(h CreateHost, req wire.QueueRequest) (wire.QueueResult, error) {
	project, epic, ok := tickets.ParseEpicAddress(req.Address)
	if !ok {
		return queue.Refusal(wire.ReasonInvalidAddress, req.Address+" is not an epic address (want project:epic)"), nil
	}
	root := queue.RootRef{Project: project, Epic: epic}
	e, ok := h.Epic(project, epic)
	if !ok || len(e.Tickets) == 0 {
		return queue.Refusal(wire.ReasonUnknownTicket, "no epic "+root.String()), nil
	}
	kind := schema.PauseReason(e.MergeBlockKind)
	if kind != schema.PauseCIFailing && kind != schema.PausePRTextFailed {
		return queue.Refusal(wire.ReasonNotPaused, root.String()+" is not paused on CI or on its pull request text"), nil
	}
	if shown := wire.EpicState(req.State); shown != "" {
		// An Epics card shows the list state; the Epic page's vocabulary calls both pauses "paused".
		if s := wire.EpicListState(shown); s == wire.EpicListCIFailing || s == wire.EpicListPRTextFailed {
			shown = wire.EpicStatePaused
		}
		if now := h.EpicState(root, e); shown != now {
			return queue.Refusal(wire.ReasonStale, fmt.Sprintf("%s is %s now, the page showed %s", root, now, shown)), nil
		}
	}
	if err := ResetBlock(e.Path, "Try again"); err != nil {
		return wire.QueueResult{}, err
	}
	if kind == schema.PausePRTextFailed {
		if _, err := Open(h, root); err != nil {
			return wire.QueueResult{}, err
		}
	}
	h.Kick()
	return wire.QueueResult{}, nil
}
