package server

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// StartOverTrunkConflict is Start over for a trunk-conflict ticket. There is no
// branch to reset: the backed-up files go back into the target's checkout, the
// ticket is done and the backup ref goes, so the epic can land. Any clash the
// restore leaves is the person's to fix by hand. A refusal comes back as a
// *repair.RefusalError.
func (s *Landing) StartOverTrunkConflict(req wire.RepairRequest, origin string, addr tickets.Address, t tickets.Ticket) (repair.StartOverResult, error) {
	if strings.TrimSpace(req.Reason) == "" {
		return repair.StartOverResult{}, &repair.RefusalError{Reason: repair.ReasonReasonRequired, Message: "--reason is required"}
	}
	if st := schema.Status(t.Status); st != schema.StatusClaimed && st != schema.StatusPaused {
		return repair.StartOverResult{}, &repair.RefusalError{Reason: repair.ReasonStatusRefused, Message: fmt.Sprintf("ticket %s is %v; start-over accepts only claimed or paused", addr.ID, st)}
	}
	e, ok := s.writer.FreshEpic(addr.Project, addr.Epic)
	if !ok || e.TrunkConflictBackup == "" {
		return repair.StartOverResult{}, &repair.RefusalError{Reason: repair.ReasonStatusRefused, Message: "no trunk-conflict backup is recorded for " + addr.Epic}
	}
	dir, err := s.targetCheckout(addr)
	if err != nil {
		return repair.StartOverResult{}, err
	}
	// Restored before anything is closed: a failure leaves the ticket as it was.
	if err := git.RestoreFromTree(dir, e.TrunkConflictBackup, e.TrunkConflictFiles); err != nil {
		return repair.StartOverResult{}, fmt.Errorf("restoring %s from %s: %w", listFiles(e.TrunkConflictFiles), e.TrunkConflictBackup, err)
	}
	if err := tickets.EndOpenRound(t.Path, origin, tickets.HowStartOver, "start-over"); err != nil {
		return repair.StartOverResult{}, err
	}
	if err := ralphloop.MarkDone(t.Path); err != nil {
		return repair.StartOverResult{}, fmt.Errorf("marking trunk-conflict ticket %s done: %w", addr.ID, err)
	}
	s.dropTrunkConflictBackup(addr)
	if _, fresh, ok := s.host.FreshTicket(addr); ok && s.guards.Merges.Begin() {
		defer s.guards.Merges.End()
		s.merger.CompleteRootIfDone(queue.RootOf(addr), ralphloop.OneIteration{Epic: addr.Epic, Ticket: fresh})
	}
	return repair.StartOverResult{Ticket: addr.ID, Status: string(schema.StatusDone)}, nil
}
