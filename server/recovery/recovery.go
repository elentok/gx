package recovery

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
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
	ref     queue.TicketRef
	log     []ralphloop.Event
	entry   recovery.Entry
	matched bool
	action  recoveryAction
	// why is an escalation's reason.
	why string
}

// holdsPause reports whether a rule remedy will run unattended, so the pause's
// chat message can wait for its outcome.
func (p recoveryPlan) holdsPause() bool { return p.action == actionRunRule }

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
	if p.why = GuardRailStop(p.log, f); p.why != "" {
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
func (rec *Recovery) planRecovery(f recovery.Failure) (recoveryPlan, bool) {
	catalog := rec.d.Settings.Load().Recovery
	if !catalog.Enabled || !f.Triggers() {
		return recoveryPlan{}, false
	}
	if f.DiagnosisOnly() {
		return rec.planEpicRecovery(f, catalog)
	}
	ref, ok, err := rec.d.Writer.FindTicket(f.Address)
	// An investigate ticket's own failure is a person's to handle, never another
	// investigation: recovery must not recurse.
	if err != nil || !ok || ref.Ticket.NoRecover || ref.Ticket.Type == string(schema.TypeInvestigate) {
		return recoveryPlan{}, false
	}
	log := rec.ticketEvents(ref, f)
	entry, matched := catalog.Match(rec.failureSequence(log, f))
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
func (rec *Recovery) planEpicRecovery(f recovery.Failure, catalog recovery.Catalog) (recoveryPlan, bool) {
	root, err := queue.ParseRootRef(f.Address)
	if err != nil {
		return recoveryPlan{}, false
	}
	projectDir, repo, err := rec.d.Projects.Of(root.Project)
	if err != nil {
		return recoveryPlan{}, false
	}
	ref := queue.TicketRef{Addr: tickets.Address{Project: root.Project, Epic: root.Epic}, ProjectDir: projectDir, Repo: repo}
	log := rec.ticketEvents(ref, f)
	entry, matched := catalog.Match(rec.failureSequence(log, f))
	plan := recoveryPlan{ref: ref, log: log, entry: entry, matched: matched}
	return plan, plan.decide(f, true)
}

// RecoverAsync starts recovery for a failure that is already written. It runs
// off the caller's goroutine: a remedy calls the same verbs the caller may be
// inside of (claim, merge), which would otherwise wait on themselves.
//
// While the start-up migrations run, it waits for them first: planning reads
// tickets and recovery starts agents, and neither may happen before they end.
func (rec *Recovery) RecoverAsync(f recovery.Failure) {
	if rec.d.Migrator.Migrating() {
		rec.d.Guards.GoSettle(func() {
			<-rec.d.Migrator.Done()
			rec.RecoverAsync(f)
		})
		return
	}
	if plan, ok := rec.planRecovery(f); ok {
		rec.d.Guards.GoSettle(func() { rec.recoverFrom(plan, f) })
	}
}

// recoverFrom runs the planned recovery through the server's own verbs.
// Failures to record are logged, never fatal: recovery is best effort on top
// of a pause that stands.
func (rec *Recovery) recoverFrom(plan recoveryPlan, f recovery.Failure) {
	if plan.matched {
		ev := ralphloop.Event{Type: string(events.RecoveryMatched), Ticket: plan.ref.Addr.ID, Kind: string(f.Kind), Reason: plan.entry.ID, Signature: plan.entry.Signature()}
		rec.appendRecoveryEvent(plan.ref, f, ev)
	}
	switch plan.action {
	case actionEscalate:
		rec.escalate(plan, f, plan.why, "")
	case actionInvestigate:
		rec.recoverByInvestigating(plan, f)
	case actionPropose:
		rec.propose(plan, f)
	case actionRunRule:
		rec.runRule(plan, f)
	}
}

// runRule runs the entry's remedy unattended. A failed remedy, or an ok one
// that leaves a held pause paused, escalates; an ok one drops the held pause
// message.
func (rec *Recovery) runRule(plan recoveryPlan, f recovery.Failure) {
	ref, entry := plan.ref, plan.entry
	outcome := "ok"
	if err := entry.Apply(f, recoveryVerbs{rec}); err != nil {
		rec.d.Log.Warn("recovery remedy failed", "entry", entry.ID, "ticket", f.Address, "err", err)
		outcome = err.Error()
	}
	rec.recordRecovery(ref, events.RecoveryApplied, f, entry.ID, outcome)
	switch {
	case outcome != "ok":
		rec.escalate(plan, f, entry.ID, outcome)
	case !rec.d.Hold.Take(f.Address):
	case rec.stillPaused(f.Address):
		// R5 and R7 end ok once their nudge is typed, which resumes nothing.
		rec.escalate(plan, f, entry.ID+" left it paused", "")
	default:
		// A --wait on the ticket reads the hold, so it must hear the hold end.
		rec.d.Events.Publish(wire.EventTicketChanged, f.Address)
	}
}

// escalate gives the failure to a person: it records recovery-escalated and
// sends its one message, naming the matched entry and the ticket that reports
// the failure. For a pause the message replaces any pause message still held.
func (rec *Recovery) escalate(plan recoveryPlan, f recovery.Failure, reason, outcome string) {
	ref := plan.ref
	rec.recordRecovery(ref, events.RecoveryEscalated, f, reason, outcome)
	if f.Type == events.Paused {
		rec.d.Hold.Take(f.Address)
		rec.d.Hold.Escalate(f.Address)
		// A --wait on the ticket reads the hold, so it must hear the escalation.
		rec.d.Events.Publish(wire.EventTicketChanged, f.Address)
	}
	why, entry, report := reason, "no match", ref.Ticket.Path
	if outcome != "" {
		why += ": " + outcome
	}
	if plan.matched {
		entry = plan.entry.ID
	}
	if report == "" {
		report = filepath.Join(ref.ProjectDir, ref.Addr.Epic)
	}
	detail := fmt.Sprintf("%s: %s\n%s\nentry: %s\nreport: %s", f.Kind, f.Reason, why, entry, report)
	rec.d.Chat.Escalated(ref.Addr.Project, rec.d.Projects.ChatOverride(ref.Addr.Project), ref.Addr.Epic, ref.Ticket.Path, ref.Addr.ID, detail)
}

// HoldPauseForRecovery starts whatever recovery the pause gets and, when a rule
// remedy will run unattended, keeps back the pause's chat message for it. It
// reports false when nothing is held: the caller then notifies at once.
func (rec *Recovery) HoldPauseForRecovery(addr tickets.Address, ticketPath string, kind events.Kind, reason string) bool {
	f := recovery.Failure{Address: addr.String(), Type: events.Paused, Kind: kind, Reason: reason}
	rec.d.Hold.Forget(f.Address)
	plan, ok := rec.planRecovery(f)
	if !ok {
		return false
	}
	if !plan.holdsPause() {
		rec.d.Hold.Start(f.Address)
		rec.d.Guards.GoSettle(func() {
			rec.recoverFrom(plan, f)
			// An opened investigation keeps it pending from here: see ticketwrite.RecoveryState.
			if rec.d.Hold.Started(f.Address) {
				rec.d.Events.Publish(wire.EventTicketChanged, f.Address)
			}
		})
		return false
	}
	rec.d.Hold.Hold(f.Address, rec.d.Settings.Load().RecoverySettings.NotifyHold, func() {
		rec.d.Events.Publish(wire.EventTicketChanged, f.Address)
		rec.d.Writer.NotifyPause(addr, ticketPath, kind, reason)
	})
	rec.d.Guards.GoSettle(func() { rec.recoverFrom(plan, f) })
	return true
}

// stillPaused re-reads the ticket: a remedy's verbs may have moved it on.
func (rec *Recovery) stillPaused(address string) bool {
	ref, ok, err := rec.d.Writer.FindTicket(address)
	if err != nil || !ok {
		return false
	}
	switch schema.Status(ref.Ticket.Status) {
	case schema.StatusPaused:
		return true
	}
	return false
}

// RecoveredCount is how many remedies in the epic's run log ended ok.
func RecoveredCount(projectDir, epic string) int {
	log, _, _ := ralphloop.ReadEvents(projectDir, epic)
	n := 0
	for _, ev := range log {
		if events.Type(ev.Type) == events.RecoveryApplied && ev.Outcome == "ok" {
			n++
		}
	}
	return n
}

const proposedRemedyHeading = "Proposed Remedy"

// propose writes what a high-authority remedy would do as an event and as a
// ticket section, then escalates: a person decides with `tickets approve`.
func (rec *Recovery) propose(plan recoveryPlan, f recovery.Failure) {
	ref, entry := plan.ref, plan.entry
	calls, err := entry.Propose(f)
	if err != nil || len(calls) == 0 {
		rec.d.Log.Warn("recovery cannot propose", "entry", entry.ID, "ticket", f.Address, "err", err)
		return
	}
	text := recovery.FormatCalls(calls)
	body := "Entry " + entry.ID + " proposes, awaiting `gx server tickets approve " + f.Address + "`:\n\n```\n" + text + "\n```"
	err = schema.UpdateTicketWithBody(ref.Ticket.Path, func(_ *schema.Ticket, b *string) {
		*b = schema.SetSection(*b, proposedRemedyHeading, body)
	})
	if err != nil {
		rec.d.Log.Warn("recovery cannot write proposal", "entry", entry.ID, "ticket", f.Address, "err", err)
		return
	}
	fingerprint, err := fileFingerprint(ref.Ticket.Path)
	if err != nil {
		rec.d.Log.Warn("recovery cannot fingerprint proposal", "entry", entry.ID, "ticket", f.Address, "err", err)
		return
	}
	ev := ralphloop.Event{Type: string(events.RecoveryProposed), Ticket: ref.Addr.ID, Kind: string(f.Kind), Reason: entry.ID, Text: text, Fingerprint: fingerprint}
	rec.appendRecoveryEvent(ref, f, ev)
	rec.d.Events.Publish(wire.EventTicketChanged, ref.Addr.String())
	rec.escalate(plan, f, entry.ID+" proposes a remedy awaiting approval", "")
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
func (rec *Recovery) ticketApprove(req wire.QueueRequest) (wire.QueueResult, error) {
	return rec.d.Writer.TicketWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		f := recovery.Failure{Address: ref.Addr.String()}
		var proposal *ralphloop.Event
		for _, ev := range rec.ticketEvents(ref, f) {
			switch events.Type(ev.Type) {
			case events.RecoveryProposed:
				proposal = &ev
			case events.RecoveryApplied:
				proposal = nil
			}
		}
		if proposal == nil {
			return queue.Refusal(wire.ReasonNoProposal, "no recovery proposal for "+f.Address), nil
		}
		fingerprint, err := fileFingerprint(ref.Ticket.Path)
		if err != nil {
			return wire.QueueResult{}, err
		}
		if fingerprint != proposal.Fingerprint {
			return queue.Refusal(wire.ReasonProposalStale, f.Address+" changed since the proposal was made"), nil
		}
		calls, err := recovery.ParseCalls(proposal.Text)
		if err != nil {
			return wire.QueueResult{}, err
		}
		f.Kind = events.Kind(proposal.Kind)
		outcome := "ok"
		if err := recovery.Run(recoveryVerbs{rec}, calls...); err != nil {
			outcome = err.Error()
		}
		rec.recordRecovery(ref, events.RecoveryApplied, f, proposal.Reason, outcome)
		if outcome != "ok" {
			return queue.Refusal(wire.ReasonRemedyFailed, outcome), nil
		}
		// Retired into Comments rather than deleted, so the ticket keeps what ran.
		err = schema.UpdateTicketWithBody(ref.Ticket.Path, func(_ *schema.Ticket, b *string) {
			*b = schema.DemoteSection(*b, "## "+proposedRemedyHeading, time.Now())
		})
		if err != nil {
			return wire.QueueResult{}, err
		}
		rec.d.Events.Publish(wire.EventTicketChanged, ref.Addr.String())
		return wire.QueueResult{}, nil
	})
}

// recoverByInvestigating is recovery for a failure that needs judgment. It
// counts as an applied recovery, so the caps and the failed-recovery check see it.
func (rec *Recovery) recoverByInvestigating(plan recoveryPlan, f recovery.Failure) {
	child, err := rec.investigate(plan.ref, f, matchedEntryText(plan.entry, plan.matched))
	if err != nil {
		rec.d.Log.Warn("recovery cannot create an investigate ticket", "ticket", f.Address, "err", err)
		rec.recordRecovery(plan.ref, events.RecoveryApplied, f, investigateEntry, err.Error())
		rec.escalate(plan, f, investigateEntry, err.Error())
		return
	}
	rec.recordRecovery(plan.ref, events.RecoveryApplied, f, investigateEntry, child)
}

// investigateRetryEntry is the reason of the recovery-applied an investigate
// agent's Retry logs; unlike investigateEntry it spends the pause kind's cap.
const investigateRetryEntry = "investigate-retry"

// AppendRecoveryApplied records the recovery an investigate retry applied.
func (rec *Recovery) AppendRecoveryApplied(ref queue.TicketRef, f recovery.Failure, investigator string) {
	rec.appendRecoveryEvent(ref, f, ralphloop.Event{
		Type: string(events.RecoveryApplied), Ticket: ref.Addr.ID, Kind: string(f.Kind),
		Reason: investigateRetryEntry, Outcome: "ok", Text: investigator,
	})
}

// InvestigateRetryFailure checks that req's investigator may Retry ref and
// returns the failure the retry answers. Unattended, it may retry any repair
// pause and a zero-commit one, within the guard rail; every other pause
// (blocked-pane included) is for a person.
func (rec *Recovery) InvestigateRetryFailure(qc queue.Ctx, ref queue.TicketRef) (recovery.Failure, wire.QueueResult, bool) {
	inv, ok, err := rec.d.Writer.FindTicket(qc.Investigator)
	if err != nil || !ok || inv.Ticket.Type != string(schema.TypeInvestigate) {
		return recovery.Failure{}, queue.Refusal(wire.ReasonInvestigateStop, qc.Investigator+" is not an investigate ticket"), false
	}
	t, err := schema.ParseTicket(ref.Ticket.Path)
	if err != nil {
		return recovery.Failure{}, queue.Refusal(wire.ReasonInvestigateStop, "cannot read "+ref.Addr.String()+": "+err.Error()), false
	}
	reason := t.EffectiveReason()
	row, hasRow := reason.Row()
	if t.PausedClass() == "" || !hasRow || !slices.Contains(row.Verbs, schema.VerbRetry) ||
		(t.PausedClass() != schema.PauseClassRepair && reason != schema.PauseZeroCommit) {
		return recovery.Failure{}, queue.Refusal(wire.ReasonInvestigateStop, ref.Addr.String()+" is "+t.Describe()+"; only a person retries it"), false
	}
	f := recovery.Failure{Address: ref.Addr.String(), Type: events.Paused, Kind: events.Kind(reason)}
	// The fork is the investigation itself, not a remedy the agent's one retry
	// of the kind could be spent on: it still counts toward the ticket's total.
	log := rec.ticketEvents(ref, f)
	for i := range log {
		if events.Type(log[i].Type) == events.RecoveryApplied && log[i].Reason == investigateEntry {
			log[i].Kind = ""
		}
	}
	if why := GuardRailStop(log, f); why != "" {
		return recovery.Failure{}, queue.Refusal(wire.ReasonInvestigateStop, why), false
	}
	return f, wire.QueueResult{}, true
}

// Guard-rail caps on automatic recovery, counted from the run log so a resume
// or a server restart cannot clear them.
const (
	maxRecoveriesPerKind   = 1
	maxRecoveriesPerTicket = 3
)

// ticketEvents is the ticket's run-log events, oldest first. An epic-level ref
// also gets every ticket's merging, reset and manual merge: progress anywhere in
// the epic clears its earlier recovery.
func (rec *Recovery) ticketEvents(ref queue.TicketRef, f recovery.Failure) []ralphloop.Event {
	log, _, err := ralphloop.ReadEvents(ref.ProjectDir, ref.Addr.Epic)
	if err != nil {
		rec.d.Log.Warn("recovery cannot read the run log", "ticket", f.Address, "err", err)
	}
	var mine []ralphloop.Event
	for _, ev := range log {
		if ev.Ticket == ref.Addr.ID || (ref.Addr.ID == "" && clearsRecovery(events.Type(ev.Type))) {
			mine = append(mine, ev)
		}
	}
	return mine
}

// GuardRailStop says why automatic recovery must not run for f, or "" when it
// may. A failure after a remedy for the same kind that no merging, reset or
// manual merge has since cleared is a failed recovery; otherwise the per-kind
// and per-ticket caps apply. An investigation or a parent backfill remedies
// nothing the next pause could be a re-failure of, so only the caps see them.
// A person's relaunch starts the budget over.
func GuardRailStop(log []ralphloop.Event, f recovery.Failure) string {
	var applied int
	perKind := map[string]int{}
	var last *ralphloop.Event
	for i, ev := range log {
		switch events.Type(ev.Type) {
		case events.HumanRelaunch:
			applied, perKind, last = 0, map[string]int{}, nil
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
	return t == events.CherryPicked || t == events.TicketStartedOver || t == events.ManualMerge
}

// failureSequence is the ticket's events as the matcher sees them, ending in
// the failure itself (already appended by the pause). A logged failure with no
// kind, like the loop's gate hold, takes the failure's. A logged failure also
// carries its session's last assistant text, which R2 and R3 read.
func (rec *Recovery) failureSequence(log []ralphloop.Event, f recovery.Failure) []recovery.Event {
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
	seq[n-1].Text = rec.lastLoggedAssistantText(log[n-1])
	return seq
}

// lastAssistantText is the last assistant text of the session a logged event
// names, or "" when it names none or the transcript cannot be read. A Codex
// agent that died before its first prompt has no rollout, which also reads "".
func (rec *Recovery) lastLoggedAssistantText(ev ralphloop.Event) string {
	if ev.AgentSession != "" && ev.Agent == ralphloop.AgentCodex {
		home, err := rec.d.Projects.CodexHome()
		if err != nil {
			return ""
		}
		text, _, _ := codexsession.LastAssistantTextIn(home, ev.AgentSession)
		return text
	}
	if ev.AgentSession == "" || ev.Cwd == "" || (ev.Agent != "" && ev.Agent != ralphloop.AgentClaude) {
		return ""
	}
	path, err := rec.d.Projects.TranscriptPath(ev.Cwd, ev.AgentSession)
	if err != nil {
		return ""
	}
	text, _, _ := transcript.LastAssistantText(path, transcript.MaxAssistantTextBytes)
	return text
}

func (rec *Recovery) recordRecovery(ref queue.TicketRef, typ events.Type, f recovery.Failure, entryID, outcome string) {
	rec.appendRecoveryEvent(ref, f, ralphloop.Event{Type: string(typ), Ticket: ref.Addr.ID, Kind: string(f.Kind), Reason: entryID, Outcome: outcome})
}

func (rec *Recovery) appendRecoveryEvent(ref queue.TicketRef, f recovery.Failure, ev ralphloop.Event) {
	if err := ralphloop.AppendEvent(ref.ProjectDir, ref.Addr.Epic, ev); err != nil {
		rec.d.Log.Warn("recovery cannot record event", "type", ev.Type, "ticket", f.Address, "err", err)
	}
}
