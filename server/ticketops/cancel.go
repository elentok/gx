package ticketops

import (
	"fmt"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop/ticketops"
	"github.com/elentok/gx/ralphloop/worktree"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// Cancel withdraws a ticket and every non-terminal ticket forked from it.
// A claimed ticket with a live pane refuses ticket-live unless Stop closes the
// pane first.
func (o *Ops) Cancel(req wire.QueueRequest) (wire.QueueResult, error) {
	return o.d.Writer.ResolvedWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		if ref.Ticket.Status == string(schema.StatusDone) {
			return queue.Refusal(wire.ReasonTicketDone, ref.Addr.String()+" is done"), nil
		}
		// An archived one-off, found by its old address, is already over.
		if ref.Addr.Epic == queue.OneOffsArchivedEpic {
			return queue.Refusal(wire.ReasonTicketDone, ref.Addr.String()+" is archived"), nil
		}
		targets := cancelTargets(ref)
		addrs := make([]tickets.Address, len(targets))
		for i, t := range targets {
			addrs[i] = tickets.Address{Project: ref.Addr.Project, Epic: ref.Addr.Epic, ID: t.Identifier}
		}
		anyLive := func(agentrunner.Runner, agentrunner.Session) bool { return true }
		// A one-off is archived on cancel, so its run can't be left alive behind it.
		stop := req.Stop || ref.Addr.Epic == queue.OneOffsEpic
		if res, refused, err := o.SettleLive(addrs, stop, wire.ReasonTicketLive, anyLive); refused || err != nil {
			return res, err
		}
		for _, t := range targets {
			if err := o.markCancelled(ref, t, req.Source); err != nil {
				return wire.QueueResult{}, err
			}
		}
		if ref.Addr.Epic == queue.OneOffsEpic {
			o.d.Index.Rescan()
		}
		return wire.QueueResult{}, nil
	})
}

// markCancelled closes t's pause round and settles it as cancelled. Its live
// agent is the caller's to stop.
func (o *Ops) markCancelled(ref queue.TicketRef, t tickets.Ticket, source string) error {
	if err := tickets.EndOpenRound(t.Path, source, tickets.HowCancel, "cancelled"); err != nil {
		return fmt.Errorf("end pause round of %s: %w", ref.Address(t), err)
	}
	if err := ticketops.SetStatus(t.Path, string(schema.StatusCancelled)); err != nil {
		return fmt.Errorf("cancel %s: %w", ref.Address(t), err)
	}
	o.d.Writer.StampFinished(tickets.Address{Project: ref.Addr.Project, Epic: ref.Addr.Epic, ID: t.Identifier}, t.Path, true)
	// The branch stays: it holds the commits.
	if ref.Repo != "" && o.awaitRunSettled(ref.Address(t)) {
		// The epic merge removes leftovers later, so a failed removal must not stop the cancel.
		if err := worktree.RemoveIterationWorktree(o.d.Projects.DefaultDeps(), ref.Repo, ref.Addr.Epic, t.Identifier); err != nil {
			o.d.Log.Warn("remove cancelled worktree", "ticket", ref.Address(t), "err", err)
		}
	}
	o.d.Events.Publish(wire.EventTicketCancelled, ref.Address(t))
	return nil
}

// awaitRunSettled waits for a stopped run to leave the registry: its finish
// still reads the worktree, and a missing one would pause the cancelled ticket.
// False means the run never settled, so its worktree is left for the epic merge.
func (o *Ops) awaitRunSettled(addr string) bool {
	return o.d.Registry.WaitGone(addr, 5*time.Second, 20*time.Millisecond)
}

// CancelInvestigateChild is a person's recovery verb withdrawing ref's
// unfinished investigate child, stopping its live agent, so an old investigation
// does not act after the person. Investigate's own verbs never call it.
func (o *Ops) CancelInvestigateChild(ref queue.TicketRef, source string) error {
	forks := ref.Epic.ForkParents()
	for _, t := range ref.Epic.Tickets {
		parent, ok := forks.Of(t)
		if !ok || parent.Identifier != ref.Ticket.Identifier || t.Type != string(schema.TypeInvestigate) || isTerminal(t.Status) {
			continue
		}
		addr := tickets.Address{Project: ref.Addr.Project, Epic: ref.Addr.Epic, ID: t.Identifier}
		anyLive := func(agentrunner.Runner, agentrunner.Session) bool { return true }
		if _, _, err := o.SettleLive([]tickets.Address{addr}, true, wire.ReasonTicketLive, anyLive); err != nil {
			return err
		}
		if err := o.markCancelled(ref, t, source); err != nil {
			return err
		}
	}
	return nil
}

// CancelInvestigateOf is CancelInvestigateChild for a ticket named by address.
func (o *Ops) CancelInvestigateOf(address, source string) error {
	ref, ok, err := o.d.Writer.FindTicket(address)
	if err != nil || !ok {
		return err
	}
	return o.CancelInvestigateChild(ref, source)
}

// SettleLive is relaunch's and cancel's one view of live agents: liveSession,
// so a leftover pane outside the registry counts too. A live session that
// blocks refuses ticket-live unless stop, which stops every blocking session
// through its runner first.
func (o *Ops) SettleLive(addrs []tickets.Address, stop bool, reason string, blocks func(agentrunner.Runner, agentrunner.Session) bool) (wire.QueueResult, bool, error) {
	type live struct {
		addr   tickets.Address
		runner agentrunner.Runner
		sess   agentrunner.Session
	}
	var found []live
	for _, a := range addrs {
		r, sess, ok, err := o.d.Harness.LiveSession(a)
		if err != nil {
			return wire.QueueResult{}, false, fmt.Errorf("find the live session of %s: %w", a, err)
		}
		if ok && blocks(r, sess) {
			found = append(found, live{a, r, sess})
		}
	}
	if len(found) > 0 && !stop {
		return queue.Refusal(reason, found[0].addr.String()+" has a live pane; use --stop"), true, nil
	}
	for _, l := range found {
		if err := l.runner.Stop(l.sess); err != nil {
			return wire.QueueResult{}, false, fmt.Errorf("stop %s: %w", l.addr, err)
		}
	}
	return wire.QueueResult{}, false, nil
}

// cancelTargets is the ticket plus its fork descendants that are not terminal.
func cancelTargets(ref queue.TicketRef) []tickets.Ticket {
	forks := ref.Epic.ForkParents()
	targets := []tickets.Ticket{ref.Ticket}
	for i := 0; i < len(targets); i++ {
		for _, t := range ref.Epic.Tickets {
			if parent, ok := forks.Of(t); ok && parent.Identifier == targets[i].Identifier && !isTerminal(t.Status) {
				targets = append(targets, t)
			}
		}
	}
	return targets
}

func isTerminal(status string) bool {
	return status == string(schema.StatusDone) || status == string(schema.StatusCancelled)
}
