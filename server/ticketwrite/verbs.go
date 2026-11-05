package ticketwrite

import (
	"fmt"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server/disk"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// PauseRequest pauses a ticket with a person's one-line reason. It refuses a
// running, done, already paused or map-epic ticket.
func (w *Writer) PauseRequest(req wire.QueueRequest) (wire.QueueResult, error) {
	reason := strings.TrimSpace(req.PauseReason)
	if reason == "" {
		return queue.Refusal(wire.ReasonReasonRequired, "a one-line reason is required"), nil
	}
	return w.TicketWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		if ref.Ticket.Status == string(schema.StatusDone) {
			return queue.Refusal(wire.ReasonTicketDone, ref.Addr.String()+" is done"), nil
		}
		switch schema.Status(ref.Ticket.Status) {
		case schema.StatusPaused, schema.StatusLegacyAnswer, schema.StatusLegacyRepair:
			return queue.Refusal(wire.ReasonAlreadyPaused, ref.Addr.String()+" is already paused"), nil
		}
		if ref.Epic.IsMap {
			return queue.Refusal(wire.ReasonMapEpic, ref.Addr.String()+" is in a map epic; its tickets are resolved by hand, not scheduled"), nil
		}
		run, _ := w.d.Registry.Tracked(ref.Addr.String())
		if err := w.Pause(ref.ProjectDir, ref.Addr, ref.Ticket.Path, events.Manual, reason, run.Run); err != nil {
			return wire.QueueResult{}, fmt.Errorf("pause %s: %w", ref.Addr, err)
		}
		return wire.QueueResult{}, nil
	})
}

// Relaunch starts a fresh iteration of a ticket outside the queue order,
// under the harness the resolver picks for it. An idle or done
// leftover pane is left to the launch, which closes a stale label holder. A
// person's relaunch logs human-relaunch, which resets the recovery budget. A
// late finish is adopted instead, with no launch.
func (w *Writer) Relaunch(req wire.QueueRequest, qc queue.Ctx) (wire.QueueResult, error) {
	h := w.d.Host
	return w.TicketWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		if ref.Ticket.Status == string(schema.StatusDone) {
			return queue.Refusal(wire.ReasonTicketDone, ref.Addr.String()+" is done"), nil
		}
		var applied *recovery.Failure
		if qc.Investigator != "" {
			f, res, ok := h.InvestigateRetryFailure(qc, ref)
			if !ok {
				return res, nil
			}
			applied = &f
			qc.Actor = recovery.ActorInvestigate
		}
		if req.State != "" {
			stale, err := PauseStale(ref.Ticket.Path, req.State)
			if err != nil {
				return wire.QueueResult{}, err
			}
			if stale {
				return queue.Refusal(wire.ReasonStale, "the pause changed; review the current one"), nil
			}
		}
		if h.AdoptFirst(ref.ProjectDir, ref.Addr, ref.Ticket.Path) {
			return wire.QueueResult{}, nil
		}
		agent := w.d.Harness.ResolveTicket(ref.Addr, ref.Ticket).Harness
		if reason, held := h.HerdrHolds(w.d.Harness.LaunchRunner(ref.Addr.Project, agent)); held {
			return queue.Refusal(wire.ReasonHerdrUnavailable, reason), nil
		}
		if res, refused := h.BudgetRefusal(); refused {
			return res, nil
		}
		if qc.Actor != "" {
			if v, held := h.UsageGate(agent, time.Now()); held {
				return queue.Refusal(wire.ReasonUsageLimit, v), nil
			}
		}
		if d, held := h.FreshHeldDisk(ref.Addr.Project); held {
			return queue.Refusal(wire.ReasonDiskLow, disk.LowReason(d)), nil
		}
		if res, refused, err := h.SettleLive([]tickets.Address{ref.Addr}, req.Stop, wire.ReasonAgentWorking, isWorking); refused || err != nil {
			return res, err
		}
		source := req.Source
		if qc.Actor != "" {
			source = tickets.SourceGx
		}
		if err := tickets.EndOpenRound(ref.Ticket.Path, source, tickets.HowRelaunch, ralphloop.CloseRelaunch); err != nil {
			return wire.QueueResult{}, fmt.Errorf("end pause round of %s: %w", ref.Addr, err)
		}
		if qc.Actor == "" {
			if err := h.CancelInvestigateChild(ref, req.Source); err != nil {
				return wire.QueueResult{}, err
			}
			err := runlog.AppendEvent(ref.ProjectDir, ref.Addr.Epic, runlog.Event{
				Time: time.Now(), Type: string(events.HumanRelaunch), Ticket: ref.Addr.ID, Address: ref.Addr.String(),
			})
			if err != nil {
				return wire.QueueResult{}, fmt.Errorf("logging relaunch of %s: %w", ref.Addr, err)
			}
		}
		epics, err := tickets.LoadResolved(ref.Addr.Project, ref.ProjectDir)
		if err != nil {
			return wire.QueueResult{}, err
		}
		if _, err := h.ClaimAndLaunch(ref.Root(), ref.Addr, ref.Ticket, ref.Repo, agent, epics); err != nil {
			return wire.QueueResult{}, err
		}
		if applied != nil {
			h.AppendRecoveryApplied(ref, *applied, qc.Investigator)
		}
		return wire.QueueResult{}, nil
	})
}

// isWorking is a session not idle or done; an unknown state counts as working,
// so it is never killed without --stop.
func isWorking(r agentrunner.Runner, sess agentrunner.Session) bool {
	st, err := r.Status(sess)
	return err != nil || !st.Waiting()
}
