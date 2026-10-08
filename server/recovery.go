package server

import (
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
)

// recoverAsync starts recovery for a failure that is already written. It runs
// off the caller's goroutine: a remedy calls the same verbs the caller may be
// inside of (claim, land), which would otherwise wait on themselves.
func (s *Server) recoverAsync(f recovery.Failure) {
	if !s.cfg.Recovery.Enabled || !f.Triggers() {
		return
	}
	go s.recoverFrom(f)
}

// recoverFrom matches the failure against the catalog and, for a rule entry,
// runs its remedy through the server's own verbs. Failures to record are
// logged, never fatal: recovery is best effort on top of a park that stands.
func (s *Server) recoverFrom(f recovery.Failure) {
	ref, ok, err := s.findTicket(f.Address)
	if err != nil || !ok || ref.ticket.NoRecover {
		return
	}
	entry, ok := s.cfg.Recovery.Match(s.failureSequence(ref, f))
	if !ok || !entry.Runnable(f) {
		return
	}
	s.recordRecovery(ref, events.RecoveryMatched, f, entry.ID, "")
	outcome := "ok"
	if err := entry.Remedy(f, recoveryVerbs{s}); err != nil {
		s.log.Warn("recovery remedy failed", "entry", entry.ID, "ticket", f.Address, "err", err)
		outcome = err.Error()
	}
	s.recordRecovery(ref, events.RecoveryApplied, f, entry.ID, outcome)
}

// failureSequence is the ticket's run-log events, oldest first, ending in the
// failure itself (already appended by the park).
func (s *Server) failureSequence(ref ticketRef, f recovery.Failure) []recovery.Event {
	log, _, err := ralphloop.ReadEvents(ref.projectDir, ref.addr.Epic)
	if err != nil {
		s.log.Warn("recovery cannot read the run log", "ticket", f.Address, "err", err)
	}
	var seq []recovery.Event
	for _, ev := range log {
		if ev.Ticket == ref.addr.ID {
			seq = append(seq, recovery.Event{Type: events.Type(ev.Type), Kind: events.Kind(ev.Kind)})
		}
	}
	if len(seq) == 0 || seq[len(seq)-1].Type != f.Type {
		seq = append(seq, recovery.Event{Type: f.Type, Kind: f.Kind})
	}
	return seq
}

func (s *Server) recordRecovery(ref ticketRef, typ events.Type, f recovery.Failure, entryID, outcome string) {
	ev := ralphloop.Event{Type: string(typ), Ticket: ref.addr.ID, Kind: string(f.Kind), Reason: entryID, Outcome: outcome}
	if err := ralphloop.AppendEvent(ref.projectDir, ref.addr.Epic, ev); err != nil {
		s.log.Warn("recovery cannot record event", "type", typ, "ticket", f.Address, "err", err)
	}
}

// recoveryVerbs lets a remedy call the server's own verbs. Each call goes
// through the same ticket write as a person's, so refusals, the land lock and
// events are identical; only the actor differs.
type recoveryVerbs struct{ s *Server }

func (v recoveryVerbs) Park(address, reason string) (recovery.Result, error) {
	res, err := v.s.ticketPark(QueueRequest{Address: address, ParkReason: reason, actor: recovery.ActorRecovery})
	return recoveryResult(res), err
}

func (v recoveryVerbs) Relaunch(address string) (recovery.Result, error) {
	res, err := v.s.ticketRelaunch(QueueRequest{Address: address, actor: recovery.ActorRecovery})
	return recoveryResult(res), err
}

func recoveryResult(r QueueResult) recovery.Result {
	return recovery.Result{Actor: recovery.ActorRecovery, Via: recovery.ViaServer, Refused: r.Refused, Reason: r.Reason, Message: r.Message}
}
