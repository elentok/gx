package server

import (
	"crypto/sha256"
	"fmt"
	"os"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// recoveryPlan is the recovery a failure gets: a catalog entry, an
// investigation, or a proposal awaiting approval.
type recoveryPlan struct {
	ref     ticketRef
	log     []ralphloop.Event
	entry   recovery.Entry
	matched bool
}

// needsJudgment reports whether the failure goes to an investigation rather
// than a rule entry.
func (p recoveryPlan) needsJudgment() bool {
	return !p.matched || p.entry.Executor == recovery.ExecutorAgent
}

// runsRemedy reports whether a rule remedy will run unattended, so the park's
// chat message can wait for its outcome.
func (p recoveryPlan) runsRemedy(f recovery.Failure) bool {
	return !p.needsJudgment() && p.entry.Runnable(f) && guardRailStop(p.log, f) == ""
}

// planRecovery reports the recovery a failure would get, if any.
func (s *Server) planRecovery(f recovery.Failure) (recoveryPlan, bool) {
	if !s.cfg.Recovery.Enabled || !f.Triggers() {
		return recoveryPlan{}, false
	}
	ref, ok, err := s.findTicket(f.Address)
	// An investigate ticket's own failure is a person's to handle, never another
	// investigation: recovery must not recurse.
	if err != nil || !ok || ref.ticket.NoRecover || ref.ticket.Type == string(schema.TypeInvestigate) {
		return recoveryPlan{}, false
	}
	log := s.ticketEvents(ref, f)
	entry, matched := s.cfg.Recovery.Match(failureSequence(log, f))
	plan := recoveryPlan{ref: ref, log: log, entry: entry, matched: matched}
	if f.DiagnosisOnly() || (!plan.needsJudgment() && !(entry.Runnable(f) || entry.Proposable(f))) {
		return recoveryPlan{}, false
	}
	return plan, true
}

// recoverAsync starts recovery for a failure that is already written. It runs
// off the caller's goroutine: a remedy calls the same verbs the caller may be
// inside of (claim, land), which would otherwise wait on themselves.
func (s *Server) recoverAsync(f recovery.Failure) {
	if plan, ok := s.planRecovery(f); ok {
		go s.recoverFrom(plan, f)
	}
}

// recoverFrom runs the planned recovery through the server's own verbs.
// Failures to record are logged, never fatal: recovery is best effort on top
// of a park that stands. A failed remedy releases the held park message
// naming the entry; a successful one drops it.
func (s *Server) recoverFrom(plan recoveryPlan, f recovery.Failure) {
	ref, entry := plan.ref, plan.entry
	if why := guardRailStop(plan.log, f); why != "" {
		s.recordRecovery(ref, events.RecoveryEscalated, f, why, "")
		return
	}
	if plan.needsJudgment() {
		s.recoverByInvestigating(ref, f, matchedEntryText(entry, plan.matched))
		return
	}
	s.recordRecovery(ref, events.RecoveryMatched, f, entry.ID, "")
	if entry.Proposable(f) {
		s.propose(ref, entry, f)
		return
	}
	outcome := "ok"
	if err := entry.Remedy(f, recoveryVerbs{s}); err != nil {
		s.log.Warn("recovery remedy failed", "entry", entry.ID, "ticket", f.Address, "err", err)
		outcome = err.Error()
	}
	s.recordRecovery(ref, events.RecoveryApplied, f, entry.ID, outcome)
	if !s.parkHold.take(f.Address) {
		return
	}
	if outcome != "ok" {
		s.parkHold.escalate(f.Address)
		s.notifyPark(ref.addr, ref.ticket.Path, f.Kind, fmt.Sprintf("%s (recovery %s failed: %s)", f.Reason, entry.ID, outcome))
	}
	// A --wait on the ticket reads the hold, so it must hear the hold end.
	s.events.publish(EventTicketChanged, f.Address)
}

// holdParkForRecovery starts whatever recovery the park gets and, when a rule
// remedy will run unattended, keeps back the park's chat message for it. It
// reports false when nothing is held: the caller then notifies at once.
func (s *Server) holdParkForRecovery(addr tickets.Address, ticketPath string, kind events.Kind, reason string) bool {
	f := recovery.Failure{Address: addr.String(), Type: kind.ParkType(), Kind: kind, Reason: reason}
	s.parkHold.forget(f.Address)
	plan, ok := s.planRecovery(f)
	if !ok {
		return false
	}
	if !plan.runsRemedy(f) {
		go s.recoverFrom(plan, f)
		return false
	}
	hold := s.cfg.RecoveryNotifyHold
	if hold <= 0 {
		hold = config.DefaultRecoveryNotifyHold
	}
	s.parkHold.hold(f.Address, hold, func() {
		s.events.publish(EventTicketChanged, f.Address)
		s.notifyPark(addr, ticketPath, kind, reason)
	})
	go s.recoverFrom(plan, f)
	return true
}

// recoveredCount is how many remedies in the epic's run log ended ok.
func recoveredCount(projectDir, epic string) int {
	log, _, _ := ralphloop.ReadEvents(projectDir, epic)
	n := 0
	for _, ev := range log {
		if events.Type(ev.Type) == events.RecoveryApplied && ev.Outcome == "ok" {
			n++
		}
	}
	return n
}

// Refusal reasons of approve.
const (
	ReasonNoProposal    = "no-proposal"
	ReasonProposalStale = "proposal-stale"
	ReasonRemedyFailed  = "remedy-failed"
)

const proposedRemedyHeading = "Proposed Remedy"

// propose writes what a high-authority remedy would do as an event and as a
// ticket section, then escalates: a person decides with `tickets approve`.
func (s *Server) propose(ref ticketRef, entry recovery.Entry, f recovery.Failure) {
	calls, err := entry.Propose(f)
	if err != nil || len(calls) == 0 {
		s.log.Warn("recovery cannot propose", "entry", entry.ID, "ticket", f.Address, "err", err)
		return
	}
	text := recovery.FormatCalls(calls)
	body := "Entry " + entry.ID + " proposes, awaiting `gx server tickets approve " + f.Address + "`:\n\n```\n" + text + "\n```"
	err = schema.UpdateTicketWithBody(ref.ticket.Path, func(_ *schema.Ticket, b *string) {
		*b = schema.SetSection(*b, proposedRemedyHeading, body)
	})
	if err != nil {
		s.log.Warn("recovery cannot write proposal", "entry", entry.ID, "ticket", f.Address, "err", err)
		return
	}
	fingerprint, err := fileFingerprint(ref.ticket.Path)
	if err != nil {
		s.log.Warn("recovery cannot fingerprint proposal", "entry", entry.ID, "ticket", f.Address, "err", err)
		return
	}
	ev := ralphloop.Event{Type: string(events.RecoveryProposed), Ticket: ref.addr.ID, Kind: string(f.Kind), Reason: entry.ID, Text: text, Fingerprint: fingerprint}
	if err := ralphloop.AppendEvent(ref.projectDir, ref.addr.Epic, ev); err != nil {
		s.log.Warn("recovery cannot record event", "type", events.RecoveryProposed, "ticket", f.Address, "err", err)
	}
	s.events.publish(EventTicketChanged, ref.addr.String())
	s.recordRecovery(ref, events.RecoveryEscalated, f, entry.ID+" proposes a remedy awaiting approval", "")
}

func fileFingerprint(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

// ticketApprove runs the ticket's latest recovery proposal, or refuses when
// there is none left to run or the ticket changed since it was proposed.
func (s *Server) ticketApprove(req QueueRequest) (QueueResult, error) {
	return s.ticketWrite(req, func(ref ticketRef) (QueueResult, error) {
		f := recovery.Failure{Address: ref.addr.String()}
		var proposal *ralphloop.Event
		for _, ev := range s.ticketEvents(ref, f) {
			switch events.Type(ev.Type) {
			case events.RecoveryProposed:
				proposal = &ev
			case events.RecoveryApplied:
				proposal = nil
			}
		}
		if proposal == nil {
			return refusal(ReasonNoProposal, "no recovery proposal for "+f.Address), nil
		}
		fingerprint, err := fileFingerprint(ref.ticket.Path)
		if err != nil {
			return QueueResult{}, err
		}
		if fingerprint != proposal.Fingerprint {
			return refusal(ReasonProposalStale, f.Address+" changed since the proposal was made"), nil
		}
		calls, err := recovery.ParseCalls(proposal.Text)
		if err != nil {
			return QueueResult{}, err
		}
		f.Kind = events.Kind(proposal.Kind)
		outcome := "ok"
		for _, c := range calls {
			res, err := c.Apply(recoveryVerbs{s})
			if err == nil && res.Refused {
				err = fmt.Errorf("%s refused: %s", c.Verb, res.Reason)
			}
			if err != nil {
				outcome = err.Error()
				break
			}
		}
		s.recordRecovery(ref, events.RecoveryApplied, f, proposal.Reason, outcome)
		if outcome != "ok" {
			return refusal(ReasonRemedyFailed, outcome), nil
		}
		return QueueResult{}, nil
	})
}

// recoverByInvestigating is recovery for a failure that needs judgment. It
// counts as an applied recovery, so the caps and the failed-recovery check see it.
func (s *Server) recoverByInvestigating(ref ticketRef, f recovery.Failure, entry string) {
	outcome, err := s.investigate(ref, f, entry)
	if err != nil {
		s.log.Warn("recovery cannot create an investigate ticket", "ticket", f.Address, "err", err)
		outcome = err.Error()
	}
	s.recordRecovery(ref, events.RecoveryApplied, f, investigateEntry, outcome)
}

// Guard-rail caps on automatic recovery, counted from the run log so a resume
// or a server restart cannot clear them.
const (
	maxRecoveriesPerKind   = 1
	maxRecoveriesPerTicket = 3
)

// ticketEvents is the ticket's run-log events, oldest first.
func (s *Server) ticketEvents(ref ticketRef, f recovery.Failure) []ralphloop.Event {
	log, _, err := ralphloop.ReadEvents(ref.projectDir, ref.addr.Epic)
	if err != nil {
		s.log.Warn("recovery cannot read the run log", "ticket", f.Address, "err", err)
	}
	var mine []ralphloop.Event
	for _, ev := range log {
		if ev.Ticket == ref.addr.ID {
			mine = append(mine, ev)
		}
	}
	return mine
}

// guardRailStop says why automatic recovery must not run for f, or "" when it
// may. A failure after a recovery that no landing, reset or manual land has
// since cleared is a failed recovery and names both failures; otherwise the
// per-kind and per-ticket caps apply.
func guardRailStop(log []ralphloop.Event, f recovery.Failure) string {
	var applied int
	perKind := map[string]int{}
	var last *ralphloop.Event
	for i, ev := range log {
		switch events.Type(ev.Type) {
		case events.RecoveryApplied:
			applied++
			perKind[ev.Kind]++
			last = &log[i]
		case events.CherryPicked, events.TicketReset, events.ManualLand:
			last = nil
		}
	}
	switch {
	case last != nil:
		return fmt.Sprintf("recovery %s for %s failed: ticket failed again with %s", last.Reason, last.Kind, f.Kind)
	case perKind[string(f.Kind)] >= maxRecoveriesPerKind:
		return fmt.Sprintf("already recovered %s %d time(s)", f.Kind, perKind[string(f.Kind)])
	case applied >= maxRecoveriesPerTicket:
		return fmt.Sprintf("already recovered this ticket %d times", applied)
	}
	return ""
}

// failureSequence is the ticket's events as the matcher sees them, ending in
// the failure itself (already appended by the park).
func failureSequence(log []ralphloop.Event, f recovery.Failure) []recovery.Event {
	var seq []recovery.Event
	for _, ev := range log {
		seq = append(seq, recovery.Event{Type: events.Type(ev.Type), Kind: events.Kind(ev.Kind)})
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
