package server

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/transcript"
)

// recoveryAction is what recovery does about a failure.
type recoveryAction int

const (
	// actionNone only records the match: a recognize entry remedies nothing.
	actionNone recoveryAction = iota
	actionInvestigate
	actionEscalate
	actionPropose
	actionRunRule
)

// recoveryPlan is the recovery a failure gets: a catalog entry, an
// investigation, a proposal awaiting approval, or an escalation.
type recoveryPlan struct {
	ref     ticketRef
	log     []ralphloop.Event
	entry   recovery.Entry
	matched bool
	action  recoveryAction
	// why is an escalation's reason.
	why string
}

// holdsPark reports whether a rule remedy will run unattended, so the park's
// chat message can wait for its outcome.
func (p recoveryPlan) holdsPark() bool { return p.action == actionRunRule }

// decide sets the plan's action, or reports false when the failure gets no
// recovery at all. diagnose marks an epic-level plan, which always needs
// judgment. The guard rail is read once, here: the log is fixed at plan time.
func (p *recoveryPlan) decide(f recovery.Failure, diagnose bool) bool {
	e := p.entry
	judgment := diagnose || !p.matched || e.Executor == recovery.ExecutorAgent
	recognize := p.matched && e.Executor == recovery.ExecutorRecognize
	acts := e.Runnable(f) || e.Proposable(f) || e.Executor == recovery.ExecutorPerson || e.Executor == recovery.ExecutorRecognize
	if (!p.matched && f.NeedsMatch()) || (!judgment && !acts) {
		return false
	}
	// Recognizing remedies nothing, so the guard rail has nothing to stop.
	if recognize {
		p.action = actionNone
		return true
	}
	if p.why = guardRailStop(p.log, f); p.why != "" {
		p.action = actionEscalate
		return true
	}
	switch {
	case judgment:
		p.action = actionInvestigate
	case e.Executor == recovery.ExecutorPerson:
		p.action, p.why = actionEscalate, e.ID+" is a person's to handle"
	case e.Proposable(f):
		p.action = actionPropose
	default:
		p.action = actionRunRule
	}
	return true
}

// planRecovery reports the recovery a failure would get, if any.
func (s *Server) planRecovery(f recovery.Failure) (recoveryPlan, bool) {
	if !s.cfg.Recovery.Enabled || !f.Triggers() {
		return recoveryPlan{}, false
	}
	if f.DiagnosisOnly() {
		return s.planEpicRecovery(f)
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
	if !plan.decide(f, false) {
		return recoveryPlan{}, false
	}
	return plan, true
}

// planEpicRecovery plans an epic-level investigation for a diagnosis-only
// failure, addressed by its root: no rule remedy runs on it, so a match only
// names the entry the investigation is told about. Its run-log events are the
// epic's own, with no ticket.
func (s *Server) planEpicRecovery(f recovery.Failure) (recoveryPlan, bool) {
	root, err := parseRootRef(f.Address)
	if err != nil {
		return recoveryPlan{}, false
	}
	projectDir, repo, err := s.projectOf(root.Project)
	if err != nil {
		return recoveryPlan{}, false
	}
	ref := ticketRef{addr: tickets.Address{Project: root.Project, Epic: root.Epic}, projectDir: projectDir, repo: repo}
	log := s.ticketEvents(ref, f)
	entry, matched := s.cfg.Recovery.Match(failureSequence(log, f))
	plan := recoveryPlan{ref: ref, log: log, entry: entry, matched: matched}
	return plan, plan.decide(f, true)
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
// of a park that stands.
func (s *Server) recoverFrom(plan recoveryPlan, f recovery.Failure) {
	if plan.matched {
		ev := ralphloop.Event{Type: string(events.RecoveryMatched), Ticket: plan.ref.addr.ID, Kind: string(f.Kind), Reason: plan.entry.ID, Signature: plan.entry.Signature()}
		s.appendRecoveryEvent(plan.ref, f, ev)
	}
	switch plan.action {
	case actionEscalate:
		s.escalate(plan, f, plan.why, "")
	case actionInvestigate:
		s.recoverByInvestigating(plan, f)
	case actionPropose:
		s.propose(plan, f)
	case actionRunRule:
		s.runRule(plan, f)
	}
}

// runRule runs the entry's remedy unattended. A failed remedy, or an ok one
// that leaves a held park parked, escalates; an ok one drops the held park
// message.
func (s *Server) runRule(plan recoveryPlan, f recovery.Failure) {
	ref, entry := plan.ref, plan.entry
	outcome := "ok"
	if err := entry.Apply(f, recoveryVerbs{s}); err != nil {
		s.log.Warn("recovery remedy failed", "entry", entry.ID, "ticket", f.Address, "err", err)
		outcome = err.Error()
	}
	s.recordRecovery(ref, events.RecoveryApplied, f, entry.ID, outcome)
	switch {
	case outcome != "ok":
		s.escalate(plan, f, entry.ID, outcome)
	case !s.parkHold.take(f.Address):
	case s.stillParked(f.Address):
		// R5 and R7 end ok once their nudge is typed, which unparks nothing.
		s.escalate(plan, f, entry.ID+" left it parked", "")
	default:
		// A --wait on the ticket reads the hold, so it must hear the hold end.
		s.events.publish(EventTicketChanged, f.Address)
	}
}

// escalate gives the failure to a person: it records recovery-escalated and
// sends its one message, naming the matched entry and the ticket that reports
// the failure. For a park the message replaces any park message still held.
func (s *Server) escalate(plan recoveryPlan, f recovery.Failure, reason, outcome string) {
	ref := plan.ref
	s.recordRecovery(ref, events.RecoveryEscalated, f, reason, outcome)
	if f.Type == events.NeedsRepair || f.Type == events.NeedsAnswer {
		s.parkHold.take(f.Address)
		s.parkHold.escalate(f.Address)
		// A --wait on the ticket reads the hold, so it must hear the escalation.
		s.events.publish(EventTicketChanged, f.Address)
	}
	why, entry, report := reason, "no match", ref.ticket.Path
	if outcome != "" {
		why += ": " + outcome
	}
	if plan.matched {
		entry = plan.entry.ID
	}
	if report == "" {
		report = filepath.Join(ref.projectDir, ref.addr.Epic)
	}
	detail := fmt.Sprintf("%s: %s\n%s\nentry: %s\nreport: %s", f.Kind, f.Reason, why, entry, report)
	s.chat.Escalated(ref.addr.Project, s.chatOverride(ref.addr.Project), ref.addr.Epic, ref.ticket.Path, ref.addr.ID, detail)
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
	if !plan.holdsPark() {
		s.parkHold.start(f.Address)
		go func() {
			s.recoverFrom(plan, f)
			// An opened investigation keeps it pending from here: see recoveryState.
			if s.parkHold.started(f.Address) {
				s.events.publish(EventTicketChanged, f.Address)
			}
		}()
		return false
	}
	s.parkHold.hold(f.Address, s.cfg.RecoverySettings.NotifyHold, func() {
		s.events.publish(EventTicketChanged, f.Address)
		s.notifyPark(addr, ticketPath, kind, reason)
	})
	go s.recoverFrom(plan, f)
	return true
}

// stillParked re-reads the ticket: a remedy's verbs may have moved it on.
func (s *Server) stillParked(address string) bool {
	ref, ok, err := s.findTicket(address)
	if err != nil || !ok {
		return false
	}
	switch schema.Status(ref.ticket.Status) {
	case schema.StatusNeedsRepair, schema.StatusNeedsAnswer:
		return true
	}
	return false
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
func (s *Server) propose(plan recoveryPlan, f recovery.Failure) {
	ref, entry := plan.ref, plan.entry
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
	s.appendRecoveryEvent(ref, f, ev)
	s.events.publish(EventTicketChanged, ref.addr.String())
	s.escalate(plan, f, entry.ID+" proposes a remedy awaiting approval", "")
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
		if err := recovery.Run(recoveryVerbs{s}, calls...); err != nil {
			outcome = err.Error()
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
func (s *Server) recoverByInvestigating(plan recoveryPlan, f recovery.Failure) {
	child, err := s.investigate(plan.ref, f, matchedEntryText(plan.entry, plan.matched))
	if err != nil {
		s.log.Warn("recovery cannot create an investigate ticket", "ticket", f.Address, "err", err)
		s.recordRecovery(plan.ref, events.RecoveryApplied, f, investigateEntry, err.Error())
		s.escalate(plan, f, investigateEntry, err.Error())
		return
	}
	s.recordRecovery(plan.ref, events.RecoveryApplied, f, investigateEntry, child)
}

// Guard-rail caps on automatic recovery, counted from the run log so a resume
// or a server restart cannot clear them.
const (
	maxRecoveriesPerKind   = 1
	maxRecoveriesPerTicket = 3
)

// ticketEvents is the ticket's run-log events, oldest first. An epic-level ref
// also gets every ticket's landing, reset and manual land: progress anywhere in
// the epic clears its earlier recovery.
func (s *Server) ticketEvents(ref ticketRef, f recovery.Failure) []ralphloop.Event {
	log, _, err := ralphloop.ReadEvents(ref.projectDir, ref.addr.Epic)
	if err != nil {
		s.log.Warn("recovery cannot read the run log", "ticket", f.Address, "err", err)
	}
	var mine []ralphloop.Event
	for _, ev := range log {
		if ev.Ticket == ref.addr.ID || (ref.addr.ID == "" && clearsRecovery(events.Type(ev.Type))) {
			mine = append(mine, ev)
		}
	}
	return mine
}

// guardRailStop says why automatic recovery must not run for f, or "" when it
// may. A failure after a remedy for the same kind that no landing, reset or
// manual land has since cleared is a failed recovery; otherwise the per-kind
// and per-ticket caps apply. An investigation or a parent backfill remedies
// nothing the next park could be a re-failure of, so only the caps see them.
func guardRailStop(log []ralphloop.Event, f recovery.Failure) string {
	var applied int
	perKind := map[string]int{}
	var last *ralphloop.Event
	for i, ev := range log {
		switch events.Type(ev.Type) {
		case events.RecoveryApplied:
			applied++
			perKind[ev.Kind]++
			if ev.Kind == string(f.Kind) && ev.Reason != investigateEntry && ev.Kind != string(events.ParentDefect) {
				last = &log[i]
			}
		}
		if clearsRecovery(events.Type(ev.Type)) {
			last = nil
		}
	}
	switch {
	case last != nil:
		return fmt.Sprintf("recovery %s for %s failed: ticket failed again", last.Reason, last.Kind)
	case perKind[string(f.Kind)] >= maxRecoveriesPerKind:
		return fmt.Sprintf("already recovered %s %d time(s)", f.Kind, perKind[string(f.Kind)])
	case applied >= maxRecoveriesPerTicket:
		return fmt.Sprintf("already recovered this ticket %d times", applied)
	}
	return ""
}

func clearsRecovery(t events.Type) bool {
	return t == events.CherryPicked || t == events.TicketReset || t == events.ManualLand
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
	s.appendRecoveryEvent(ref, f, ralphloop.Event{Type: string(typ), Ticket: ref.addr.ID, Kind: string(f.Kind), Reason: entryID, Outcome: outcome})
}

func (s *Server) appendRecoveryEvent(ref ticketRef, f recovery.Failure, ev ralphloop.Event) {
	if err := ralphloop.AppendEvent(ref.projectDir, ref.addr.Epic, ev); err != nil {
		s.log.Warn("recovery cannot record event", "type", ev.Type, "ticket", f.Address, "err", err)
	}
}

// recoveryVerbs lets a remedy call the server's own verbs. Each call goes
// through the same ticket write as a person's, so refusals, the land lock and
// events are identical; only the actor differs.
type recoveryVerbs struct{ s *Server }

// recoveryVerbHandlers are the server's verbs by name; each takes the request
// a call makes, already stamped actor recovery.
var recoveryVerbHandlers = map[string]func(s *Server, req QueueRequest, c recovery.Call) (QueueResult, error){
	recovery.VerbPark: func(s *Server, req QueueRequest, c recovery.Call) (QueueResult, error) {
		req.ParkReason = c.Text
		return s.ticketPark(req)
	},
	recovery.VerbRelaunch: func(s *Server, req QueueRequest, _ recovery.Call) (QueueResult, error) {
		return s.ticketRelaunch(req)
	},
	recovery.VerbCommitlessDone: func(s *Server, req QueueRequest, _ recovery.Call) (QueueResult, error) {
		return s.ticketCommitlessDone(req)
	},
	recovery.VerbNudge: func(s *Server, req QueueRequest, c recovery.Call) (QueueResult, error) {
		req.Text = c.Text
		return s.ticketNudge(req)
	},
	recovery.VerbClosePane:   (*Server).recoveryClosePane,
	recovery.VerbWait:        (*Server).recoveryWait,
	recovery.VerbReleaseGate: (*Server).recoveryReleaseGate,
	recovery.VerbFinish:      (*Server).recoveryFinish,
	recovery.VerbSetParent:   (*Server).recoverySetParent,
}

func (v recoveryVerbs) Do(c recovery.Call) (recovery.Result, error) {
	h, ok := recoveryVerbHandlers[c.Verb]
	if !ok {
		return recovery.Result{}, fmt.Errorf("unknown recovery verb %q", c.Verb)
	}
	res, err := h(v.s, QueueRequest{Address: c.Address, actor: recovery.ActorRecovery}, c)
	return recoveryResult(res), err
}

func (s *Server) recoveryClosePane(req QueueRequest, _ recovery.Call) (QueueResult, error) {
	return s.resolvedWrite(req, func(ref ticketRef) (QueueResult, error) {
		it, live := s.liveIteration(ref.addr.String())
		if !live {
			return QueueResult{}, nil
		}
		if err := herdr.TabClose(it.Tab); err != nil {
			return QueueResult{}, fmt.Errorf("close pane of %s: %w", ref.addr, err)
		}
		return QueueResult{}, nil
	})
}

// compactRewaitTimeoutMs is R7's one extended wait, well past the loop's own
// compaction wait so a slow compaction can still finish inside it.
const compactRewaitTimeoutMs = 15 * 60 * 1000

func (s *Server) recoveryWait(req QueueRequest, _ recovery.Call) (QueueResult, error) {
	return s.resolvedWrite(req, func(ref ticketRef) (QueueResult, error) {
		if _, live := s.liveIteration(ref.addr.String()); !live {
			return refusal(ReasonIterationNotLive, "no live iteration for "+ref.addr.String()), nil
		}
		label, _, _ := ralphloop.IterationIdentity(ref.addr.Epic, ref.addr.ID, "")
		if _, err := herdr.AgentWait(herdr.AgentWaitOptions{Target: label, Until: []string{"idle", "done"}, TimeoutMs: compactRewaitTimeoutMs}); err != nil {
			return QueueResult{}, fmt.Errorf("wait for %s: %w", ref.addr, err)
		}
		return QueueResult{}, nil
	})
}

// Refusal reasons of release-gate and finish.
const (
	ReasonAgentBusy      = "agent-busy"
	ReasonWorktreeDirty  = "worktree-dirty"
	ReasonNoCommitsAhead = "no-commits-ahead"
	ReasonFinishTimedOut = "finish-timed-out"
)

func (s *Server) recoveryReleaseGate(req QueueRequest, _ recovery.Call) (QueueResult, error) {
	return s.resolvedWrite(req, func(ref ticketRef) (QueueResult, error) {
		run, ok := s.registry.tracked(ref.addr.String())
		if !ok {
			return refusal(ReasonIterationNotLive, "no live iteration for "+ref.addr.String()), nil
		}
		label, _, worktree := ralphloop.IterationIdentity(ref.addr.Epic, ref.addr.ID, s.worktreeDir(ref.addr.Project))
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
		s.registry.releaseGate(ref.addr.String())
		return QueueResult{}, nil
	})
}

// finishWaitTimeout bounds Finish: a released gate's finish only re-checks
// idle and lands, so a run still live well past that is stuck elsewhere.
const finishWaitTimeout = 10 * time.Minute

// finishPoll paces Finish's registry reads; a var so tests can shorten it.
var finishPoll = time.Second

// recoveryFinish holds no ticket lock while it waits: the land it waits for
// needs it.
func (s *Server) recoveryFinish(req QueueRequest, _ recovery.Call) (QueueResult, error) {
	for deadline := time.Now().Add(finishWaitTimeout); s.registry.has(req.Address); time.Sleep(finishPoll) {
		if time.Now().After(deadline) {
			return refusal(ReasonFinishTimedOut, req.Address+" is still running after its gate was released"), nil
		}
	}
	return QueueResult{}, nil
}

// Refusal reasons of set-parent.
const (
	ReasonParentChanged = "parent-changed"
	ReasonInvalidParent = "invalid-parent"
)

// recoverySetParent re-checks and writes under the epic lock `gx tickets set
// --parent` takes, so a re-parent between the scan and the write is seen, not
// clobbered.
func (s *Server) recoverySetParent(req QueueRequest, c recovery.Call) (QueueResult, error) {
	address, parent := req.Address, c.Parent
	return s.resolvedWrite(req, func(ref ticketRef) (QueueResult, error) {
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
