package server

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

const investigateEntry = "investigate"

// investigate forks a commitless investigate ticket off the failed ticket and
// queues it at the front. The parent stays parked; it waits on the child through
// the ordinary parent edge. It returns the child's address.
func (s *Server) investigate(ref ticketRef, f recovery.Failure) (string, error) {
	path, id, err := writeInvestigateTicket(ref, f)
	if err != nil {
		return "", err
	}
	child := tickets.Address{Project: ref.addr.Project, Epic: ref.addr.Epic, ID: id}
	// queueAdd checks the index, so it must see the new ticket first.
	if err := s.idx.refresh(s.cfg.TicketStore); err != nil {
		return "", fmt.Errorf("refresh after writing %s: %w", path, err)
	}
	res, err := s.queueAdd(QueueRequest{Address: child.String(), Front: true, actor: recovery.ActorRecovery})
	if err != nil {
		return "", err
	}
	if res.Refused {
		return "", fmt.Errorf("queue %s: %s", child, res.Message)
	}
	return child.String(), nil
}

func writeInvestigateTicket(ref ticketRef, f recovery.Failure) (path, id string, err error) {
	epicPath := filepath.Join(ref.projectDir, ref.addr.Epic)
	epic, unlock, err := tickets.LoadLockedEpic(epicPath)
	if err != nil {
		return "", "", fmt.Errorf("load epic %s: %w", epicPath, err)
	}
	defer unlock()

	id, err = tickets.NextTicketID(*epic, ref.addr.ID)
	if err != nil {
		return "", "", fmt.Errorf("allocate investigate id: %w", err)
	}
	parent := schema.TicketID(ref.addr.ID)
	child := schema.Ticket{ID: schema.TicketID(id), Status: schema.StatusOpen, Type: schema.TypeInvestigate, Parent: &parent}
	body := fmt.Sprintf(
		"\n# %s — Investigate %s\n\n## What to build\n\nTicket %s parked with %s (%s): %s\n\nFind out why and fix it or report what a person must do.\n\n## Acceptance criteria\n\n- [ ] Cause found and the parked ticket recovered, or the findings reported\n",
		id, parent, parent, f.Type, f.Kind, f.Reason,
	)
	out, err := schema.MarshalTicket(child, body)
	if err != nil {
		return "", "", fmt.Errorf("marshal investigate ticket %s: %w", id, err)
	}
	path = filepath.Join(epicPath, "issues", fmt.Sprintf("%s-investigate.md", id))
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return "", "", fmt.Errorf("write %s: %w", path, err)
	}
	if _, err := schema.ParseTicket(path); err != nil {
		return "", "", fmt.Errorf("investigate ticket %s failed validation: %w", path, err)
	}
	return path, id, nil
}
