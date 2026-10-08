package server

import (
	"crypto/sha256"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/transcript"
)

// recoveryPlan is the recovery a failure gets: a catalog entry, an
// investigation, a proposal awaiting approval, or an escalation.
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
	acts := entry.Runnable(f) || entry.Proposable(f) || entry.Executor == recovery.ExecutorPerson
	if f.DiagnosisOnly() || (!matched && f.NeedsMatch()) || (!plan.needsJudgment() && !acts) {
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
	if entry.Executor == recovery.ExecutorPerson {
		s.recordRecovery(ref, events.RecoveryEscalated, f, entry.ID+" is a person's to handle", "")
		return
	}
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
	if outcome != "ok" && f.Type != events.NeedsRepair && f.Type != events.NeedsAnswer {
		// No park holds a message for this failure, so nothing else tells a
		// person the remedy failed.
		s.recordRecovery(ref, events.RecoveryEscalated, f, entry.ID, outcome)
		return
	}
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
		// Retired into Comments rather than deleted, so the ticket keeps what ran.
		err = schema.UpdateTicketWithBody(ref.ticket.Path, func(_ *schema.Ticket, b *string) {
			*b = schema.DemoteSection(*b, "## "+proposedRemedyHeading, time.Now())
		})
		if err != nil {
			return QueueResult{}, err
		}
		s.events.publish(EventTicketChanged, ref.addr.String())
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
// the failure itself (already appended by the park). A logged failure with no
// kind, like the loop's gate hold, takes the failure's. A logged failure also
// carries its session's last assistant text, which R2 and R3 read.
func failureSequence(log []ralphloop.Event, f recovery.Failure) []recovery.Event {
	var seq []recovery.Event
	for _, ev := range log {
		seq = append(seq, recovery.Event{Type: events.Type(ev.Type), Kind: events.Kind(ev.Kind), Reason: ev.Reason, Time: ev.Time})
	}
	n := len(seq)
	if n == 0 || seq[n-1].Type != f.Type {
		return append(seq, recovery.Event{Type: f.Type, Kind: f.Kind, Reason: f.Reason, Time: time.Now()})
	}
	if seq[n-1].Kind == "" {
		seq[n-1].Kind = f.Kind
	}
	seq[n-1].Text = lastAssistantText(log[n-1])
	return seq
}

// lastAssistantText is the last assistant text of the session a logged event
// names, or "" when it names none or the transcript cannot be read. Only
// Claude transcripts are read.
func lastAssistantText(ev ralphloop.Event) string {
	if ev.AgentSession == "" || ev.Cwd == "" || (ev.Agent != "" && ev.Agent != ralphloop.AgentClaude) {
		return ""
	}
	path, err := transcript.Path(ev.Cwd, ev.AgentSession)
	if err != nil {
		return ""
	}
	text, _, _ := transcript.LastAssistantText(path)
	return text
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

func (v recoveryVerbs) CommitlessDone(address string) (recovery.Result, error) {
	res, err := v.s.ticketCommitlessDone(QueueRequest{Address: address, actor: recovery.ActorRecovery})
	return recoveryResult(res), err
}

func (v recoveryVerbs) Nudge(address, text string) (recovery.Result, error) {
	res, err := v.s.ticketNudge(QueueRequest{Address: address, Text: text, actor: recovery.ActorRecovery})
	return recoveryResult(res), err
}

func (v recoveryVerbs) ClosePane(address string) (recovery.Result, error) {
	res, err := v.s.resolvedWrite(QueueRequest{Address: address, actor: recovery.ActorRecovery}, func(ref ticketRef) (QueueResult, error) {
		it, live := v.s.liveIteration(ref.addr.String())
		if !live {
			return QueueResult{}, nil
		}
		if err := herdr.TabClose(it.Tab); err != nil {
			return QueueResult{}, fmt.Errorf("close pane of %s: %w", ref.addr, err)
		}
		return QueueResult{}, nil
	})
	return recoveryResult(res), err
}

// compactRewaitTimeoutMs is R7's one extended wait, well past the loop's own
// compaction wait so a slow compaction can still finish inside it.
const compactRewaitTimeoutMs = 15 * 60 * 1000

func (v recoveryVerbs) Wait(address string) (recovery.Result, error) {
	res, err := v.s.resolvedWrite(QueueRequest{Address: address, actor: recovery.ActorRecovery}, func(ref ticketRef) (QueueResult, error) {
		if _, live := v.s.liveIteration(ref.addr.String()); !live {
			return refusal(ReasonIterationNotLive, "no live iteration for "+ref.addr.String()), nil
		}
		label, _, _ := ralphloop.IterationIdentity(ref.addr.Epic, ref.addr.ID, "")
		if _, err := herdr.AgentWait(herdr.AgentWaitOptions{Target: label, Until: []string{"idle", "done"}, TimeoutMs: compactRewaitTimeoutMs}); err != nil {
			return QueueResult{}, fmt.Errorf("wait for %s: %w", ref.addr, err)
		}
		return QueueResult{}, nil
	})
	return recoveryResult(res), err
}

// Refusal reasons of release-gate and finish.
const (
	ReasonAgentBusy      = "agent-busy"
	ReasonWorktreeDirty  = "worktree-dirty"
	ReasonNoCommitsAhead = "no-commits-ahead"
	ReasonFinishTimedOut = "finish-timed-out"
)

func (v recoveryVerbs) ReleaseGate(address string) (recovery.Result, error) {
	res, err := v.s.resolvedWrite(QueueRequest{Address: address, actor: recovery.ActorRecovery}, func(ref ticketRef) (QueueResult, error) {
		run, ok := v.s.registry.tracked(ref.addr.String())
		if !ok {
			return refusal(ReasonIterationNotLive, "no live iteration for "+ref.addr.String()), nil
		}
		label, _, worktree := ralphloop.IterationIdentity(ref.addr.Epic, ref.addr.ID, v.s.worktreeDir(ref.addr.Project))
		agent, err := herdr.AgentGet(label)
		if err != nil {
			return QueueResult{}, fmt.Errorf("read agent of %s: %w", ref.addr, err)
		}
		if agent.AgentStatus != "idle" && agent.AgentStatus != "done" {
			return refusal(ReasonAgentBusy, ref.addr.String()+" agent is "+agent.AgentStatus), nil
		}
		staged, unstaged, untracked, err := git.WorktreeStatusSummary(worktree)
		if err != nil {
			return QueueResult{}, fmt.Errorf("status of %s: %w", worktree, err)
		}
		if staged+unstaged+untracked > 0 {
			return refusal(ReasonWorktreeDirty, worktree+" has uncommitted changes"), nil
		}
		ahead, err := git.CommitsAhead(worktree, run.Base, "HEAD")
		if err != nil {
			return QueueResult{}, fmt.Errorf("commits ahead in %s: %w", worktree, err)
		}
		if ahead < 1 {
			return refusal(ReasonNoCommitsAhead, worktree+" has no commits ahead of its base"), nil
		}
		v.s.registry.releaseGate(ref.addr.String())
		return QueueResult{}, nil
	})
	return recoveryResult(res), err
}

// finishWaitTimeout bounds Finish: a released gate's finish only re-checks
// idle and lands, so a run still live well past that is stuck elsewhere.
const finishWaitTimeout = 10 * time.Minute

// finishPoll paces Finish's registry reads; a var so tests can shorten it.
var finishPoll = time.Second

// Finish holds no ticket lock while it waits: the land it waits for needs it.
func (v recoveryVerbs) Finish(address string) (recovery.Result, error) {
	for deadline := time.Now().Add(finishWaitTimeout); v.s.registry.has(address); time.Sleep(finishPoll) {
		if time.Now().After(deadline) {
			return recoveryResult(refusal(ReasonFinishTimedOut, address+" is still running after its gate was released")), nil
		}
	}
	return recoveryResult(QueueResult{}), nil
}

// Refusal reasons of set-parent.
const (
	ReasonParentChanged = "parent-changed"
	ReasonInvalidParent = "invalid-parent"
)

// SetParent re-checks and writes under the epic lock `gx tickets set --parent`
// takes, so a re-parent between the scan and the write is seen, not clobbered.
func (v recoveryVerbs) SetParent(address, parent string) (recovery.Result, error) {
	res, err := v.s.resolvedWrite(QueueRequest{Address: address, actor: recovery.ActorRecovery}, func(ref ticketRef) (QueueResult, error) {
		epic, unlock, err := tickets.LoadLockedEpic(ref.epic.Path)
		if err != nil {
			return QueueResult{}, fmt.Errorf("lock epic of %s: %w", ref.addr, err)
		}
		defer unlock()
		i := slices.IndexFunc(epic.Tickets, func(t tickets.Ticket) bool { return t.Identifier == ref.addr.ID })
		if i < 0 {
			return refusal(ReasonUnknownTicket, "no ticket "+address), nil
		}
		target := &epic.Tickets[i]
		if _, defect := recovery.ParentDefect(target.Identifier, target.Parent); !defect {
			return refusal(ReasonParentChanged, address+" has a parent its ID allows"), nil
		}
		target.Parent = &parent
		if err := epic.ValidateParentGraph(); err != nil {
			return refusal(ReasonInvalidParent, err.Error()), nil
		}
		err = schema.UpdateTicket(target.Path, func(t *schema.Ticket) {
			id := schema.TicketID(parent)
			t.Parent = &id
		})
		if err != nil {
			return QueueResult{}, fmt.Errorf("set parent of %s: %w", ref.addr, err)
		}
		return QueueResult{}, nil
	})
	return recoveryResult(res), err
}

func (v recoveryVerbs) LaunchPrompt(address string) (string, error) {
	ref, ok, err := v.s.findTicket(address)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("no ticket %s", address)
	}
	return launchPrompt(v.s.queuedAgent(address), launchSkill(ref.ticket), investigatePrompt(ref.ticket), address), nil
}

// ticketCommitlessDone marks a ticket done with no commits of its own, for work
// that landed inside another ticket's commits. Commitless takes it out of
// reconcile's hands for good, which is why only an approved proposal calls it.
func (s *Server) ticketCommitlessDone(req QueueRequest) (QueueResult, error) {
	return s.ticketWrite(req, func(ref ticketRef) (QueueResult, error) {
		err := schema.UpdateTicket(ref.ticket.Path, func(t *schema.Ticket) {
			t.Status, t.IterationStatus, t.Commitless = schema.StatusDone, schema.IterationStatusFinished, true
		})
		if err != nil {
			return QueueResult{}, fmt.Errorf("commitless-done %s: %w", ref.addr, err)
		}
		s.events.publish(EventTicketDone, ref.addr.String())
		return QueueResult{}, nil
	})
}

func recoveryResult(r QueueResult) recovery.Result {
	return recovery.Result{Actor: recovery.ActorRecovery, Via: recovery.ViaServer, Refused: r.Refused, Reason: r.Reason, Message: r.Message}
}
