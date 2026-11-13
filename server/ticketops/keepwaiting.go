package ticketops

import (
	"fmt"
	"slices"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/ralphloop/ticketops"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketlog"
)

// KeepWaiting puts the live run of a background-task-gate pause back in the
// finish wait. The wait starts over with a fresh cap; no agent is launched and
// nothing is killed. A task that has ended meanwhile releases the wait as usual.
func (o *Ops) KeepWaiting(req wire.QueueRequest) (wire.QueueResult, error) {
	return o.gateVerb(req, "kept waiting by a person", nil)
}

// gateVerb answers a background-task-gate pause on its live run: act (when
// set) runs first and may refuse, then the pause round ends, a fresh held
// event is logged and the finish wait starts over with a fresh cap.
func (o *Ops) gateVerb(req wire.QueueRequest, heldReason string, act func(run runs.TrackedRun, taskID string) (*wire.QueueResult, error)) (wire.QueueResult, error) {
	return o.d.Writer.ResolvedWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		address := ref.Addr.String()
		t, err := schema.ParseTicket(ref.Ticket.Path)
		if err != nil {
			return wire.QueueResult{}, err
		}
		if t.PausedClass() == "" || t.PauseReason != schema.PauseBackgroundTaskGate {
			return queue.Refusal(wire.ReasonNotPaused, address+" is not paused on a background task"), nil
		}
		if req.State != "" {
			stale, err := ticketwrite.PauseStale(ref.Ticket.Path, req.State)
			if err != nil {
				return wire.QueueResult{}, err
			}
			if stale {
				return queue.Refusal(wire.ReasonStale, "the pause changed; review the current one"), nil
			}
		}
		run, ok := o.d.Registry.Tracked(address)
		if !ok {
			return queue.Refusal(wire.ReasonAgentGone, address+" has no live agent to keep waiting for"), nil
		}
		taskID, err := pausedTaskID(ref)
		if err != nil {
			return wire.QueueResult{}, err
		}
		if act != nil {
			refusal, err := act(run, taskID)
			if err != nil || refusal != nil {
				if refusal != nil {
					return *refusal, nil
				}
				return wire.QueueResult{}, err
			}
		}
		if err := ticketlog.EndOpenRound(ref.Ticket.Path, req.Source, ticketlog.HowResume, ticketops.CloseResumed); err != nil {
			return wire.QueueResult{}, fmt.Errorf("end pause round of %s: %w", ref.Addr, err)
		}
		held := run.Run.PauseEvent()
		held.Time, held.Type = time.Now(), string(events.BackgroundTaskGateHeld)
		held.Ticket, held.Address, held.TaskID = ref.Addr.ID, address, taskID
		held.Reason = "background task " + taskID + ": " + heldReason
		if err := runlog.AppendEvent(ref.ProjectDir, ref.Addr.Epic, held); err != nil {
			return wire.QueueResult{}, fmt.Errorf("log held gate of %s: %w", ref.Addr, err)
		}
		if err := ticketops.Claim(ref.Ticket.Path); err != nil {
			return wire.QueueResult{}, fmt.Errorf("claim %s: %w", ref.Addr, err)
		}
		if err := o.d.Launch.ResettleWaiting(run); err != nil {
			return wire.QueueResult{}, fmt.Errorf("settle %s: %w", ref.Addr, err)
		}
		o.d.Index.Rescan()
		return wire.QueueResult{}, nil
	})
}

// pausedTaskID is the task the latest background-task-gate pause of ref named.
func pausedTaskID(ref queue.TicketRef) (string, error) {
	log, _, err := runlog.ReadEvents(ref.ProjectDir, ref.Addr.Epic)
	if err != nil {
		return "", err
	}
	for _, ev := range slices.Backward(log) {
		if ev.Ticket == ref.Addr.ID && events.Type(ev.Type) == events.Paused && ev.Kind == string(events.BackgroundTaskGate) {
			return ev.TaskID, nil
		}
	}
	return "", nil
}
