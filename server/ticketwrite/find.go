package ticketwrite

import (
	"fmt"
	"path/filepath"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// FindTicket resolves a canonical address to its ticket, or reports it unknown.
func (w *Writer) FindTicket(address string) (queue.TicketRef, bool, error) {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return queue.TicketRef{}, false, err
	}
	projectDir, repo, err := w.d.Projects.Of(addr.Project)
	if err != nil {
		return queue.TicketRef{}, false, nil
	}
	epics, err := tickets.LoadResolved(addr.Project, projectDir)
	if err != nil {
		return queue.TicketRef{}, false, err
	}
	find := func(addr tickets.Address) (queue.TicketRef, bool) {
		for _, e := range epics {
			if filepath.Base(e.Path) != addr.Epic {
				continue
			}
			for _, t := range e.Tickets {
				if t.Identifier == addr.ID {
					return queue.TicketRef{Addr: addr, ProjectDir: projectDir, Repo: repo, Ticket: t, Epic: e}, true
				}
			}
		}
		return queue.TicketRef{}, false
	}
	if ref, ok := find(addr); ok {
		return ref, true, nil
	}
	// An archived one-off is still reachable by the address it was moved from.
	moved, _, err := tickets.FindOriginal(addr.Project, projectDir, addr)
	if err != nil {
		return queue.TicketRef{}, false, nil
	}
	ref, ok := find(moved)
	return ref, ok, nil
}

// TicketWrite resolves the request's ticket and applies the refusals every
// ticket write shares, before do runs.
func (w *Writer) TicketWrite(req wire.QueueRequest, do func(queue.TicketRef) (wire.QueueResult, error)) (wire.QueueResult, error) {
	return w.ResolvedWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		if w.d.Registry.Has(ref.Addr.String()) {
			return queue.Refusal(wire.ReasonIterationRunning, "an iteration is running in "+ref.Addr.String()), nil
		}
		return do(ref)
	})
}

// ResolvedWrite is TicketWrite without the running-iteration refusal, for
// writes that judge liveness themselves.
func (w *Writer) ResolvedWrite(req wire.QueueRequest, do func(queue.TicketRef) (wire.QueueResult, error)) (wire.QueueResult, error) {
	addr, bad := queue.Canonical(req.Address)
	if bad != nil {
		return *bad, nil
	}
	ref, ok, err := w.FindTicket(addr)
	if err != nil {
		return wire.QueueResult{}, err
	}
	if !ok {
		return queue.Refusal(wire.ReasonUnknownTicket, "no ticket "+addr), nil
	}
	return do(ref)
}

// PauseStale reports whether state no longer names the open pause round of the
// ticket stored at ticketPath.
func PauseStale(ticketPath, state string) (bool, error) {
	t, err := schema.ParseTicket(ticketPath)
	if err != nil {
		return false, err
	}
	evs, err := tickets.ReadLog(ticketPath)
	if err != nil {
		return false, fmt.Errorf("read log of %s: %w", ticketPath, err)
	}
	open := tickets.OpenRound(evs)
	return t.PausedClass() == "" || open == nil || queue.AnswerState(*open, t.PausedClass()) != state, nil
}
