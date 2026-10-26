package server

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// startOverTrunkConflict is Start over for a trunk-conflict ticket. There is no
// branch to reset: the backed-up files go back into the target's checkout, the
// ticket is done and the backup ref goes, so the epic can land. Any clash the
// restore leaves is the person's to fix by hand.
func (s *Server) startOverTrunkConflict(req RepairRequest, addr tickets.Address, t tickets.Ticket) (RepairResult, error) {
	if strings.TrimSpace(req.Reason) == "" {
		return repairRefusal(&repair.RefusalError{Reason: repair.ReasonReasonRequired, Message: "--reason is required"}), nil
	}
	if st := schema.Status(t.Status); st != schema.StatusClaimed && st != schema.StatusPaused {
		return repairRefusal(&repair.RefusalError{Reason: repair.ReasonStatusRefused, Message: fmt.Sprintf("ticket %s is %v; start-over accepts only claimed or paused", addr.ID, st)}), nil
	}
	e, ok := s.freshEpic(addr.Project, addr.Epic)
	if !ok || e.TrunkConflictBackup == "" {
		return repairRefusal(&repair.RefusalError{Reason: repair.ReasonStatusRefused, Message: "no trunk-conflict backup is recorded for " + addr.Epic}), nil
	}
	dir, err := s.targetCheckout(addr)
	if err != nil {
		return RepairResult{}, err
	}
	// Restored before anything is closed: a failure leaves the ticket as it was.
	if err := git.RestoreFromTree(dir, e.TrunkConflictBackup, e.TrunkConflictFiles); err != nil {
		return RepairResult{}, fmt.Errorf("restoring %s from %s: %w", listFiles(e.TrunkConflictFiles), e.TrunkConflictBackup, err)
	}
	if err := tickets.EndOpenRound(t.Path, req.origin, tickets.HowStartOver, "start-over"); err != nil {
		return RepairResult{}, err
	}
	if err := ralphloop.MarkDone(t.Path); err != nil {
		return RepairResult{}, fmt.Errorf("marking trunk-conflict ticket %s done: %w", addr.ID, err)
	}
	s.dropTrunkConflictBackup(addr)
	if _, fresh, ok := s.freshTicket(addr); ok && s.merges.begin() {
		defer s.merges.end()
		s.completeRootIfDone(rootOf(addr), ralphloop.OneIteration{Epic: addr.Epic, Ticket: fresh})
	}
	return repairOK(repair.StartOverResult{Ticket: addr.ID, Status: string(schema.StatusDone)})
}
