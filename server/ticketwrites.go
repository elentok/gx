package server

import (
	"fmt"
	"github.com/elentok/gx/events"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// Refusal reasons of the ticket writes (pause, relaunch, unmute).
const (
	ReasonReasonRequired   = "reason-required"
	ReasonAlreadyPaused    = "already-paused"
	ReasonNotMuted         = "not-muted"
	ReasonIterationRunning = "iteration-running"
	ReasonTicketDone       = "ticket-done"
	ReasonHerdrUnavailable = "herdr-unavailable"
	// ReasonAgentWorking refuses a retry while the ticket's agent is still
	// working; the web maps it to "Stop agent and retry".
	ReasonAgentWorking = "agent-working"
	// ReasonBudgetLimit refuses a relaunch or nudge past the daily soft
	// limit (the hard limit implies it), as claims are held back.
	ReasonBudgetLimit = "budget-limit"
)

// budgetRefusal refuses new agent work past the daily soft limit; ok is
// false when there is nothing to refuse.
func (s *Server) budgetRefusal() (QueueResult, bool) {
	if !s.budgetSoftReached(time.Now()) {
		return QueueResult{}, false
	}
	return refusal(ReasonBudgetLimit, "the daily budget limit is reached; raise it with `gx budget override`"), true
}

// ticketRef is a ticket found in the store, with where it lives.
type ticketRef struct {
	addr       tickets.Address
	projectDir string
	repo       string
	ticket     tickets.Ticket
	epic       tickets.Epic
}

func (r ticketRef) root() rootRef { return rootOf(r.addr) }

// findTicket resolves a canonical address to its ticket, or reports it unknown.
func (s *Server) findTicket(address string) (ticketRef, bool, error) {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return ticketRef{}, false, err
	}
	projectDir, repo, err := s.projectOf(addr.Project)
	if err != nil {
		return ticketRef{}, false, nil
	}
	epics, err := tickets.LoadResolved(addr.Project, projectDir)
	if err != nil {
		return ticketRef{}, false, err
	}
	find := func(addr tickets.Address) (ticketRef, bool) {
		for _, e := range epics {
			if filepath.Base(e.Path) != addr.Epic {
				continue
			}
			for _, t := range e.Tickets {
				if t.Identifier == addr.ID {
					return ticketRef{addr: addr, projectDir: projectDir, repo: repo, ticket: t, epic: e}, true
				}
			}
		}
		return ticketRef{}, false
	}
	if ref, ok := find(addr); ok {
		return ref, true, nil
	}
	// An archived one-off is still reachable by the address it was moved from.
	moved, _, err := tickets.FindOriginal(addr.Project, projectDir, addr)
	if err != nil {
		return ticketRef{}, false, nil
	}
	ref, ok := find(moved)
	return ref, ok, nil
}

// ticketWrite resolves the request's ticket and applies the refusals every
// ticket write shares, before do runs.
func (s *Server) ticketWrite(req QueueRequest, do func(ticketRef) (QueueResult, error)) (QueueResult, error) {
	return s.resolvedWrite(req, func(ref ticketRef) (QueueResult, error) {
		if s.registry.has(ref.addr.String()) {
			return refusal(ReasonIterationRunning, "an iteration is running in "+ref.addr.String()), nil
		}
		return do(ref)
	})
}

// resolvedWrite is ticketWrite without the running-iteration refusal, for
// writes that judge liveness themselves.
func (s *Server) resolvedWrite(req QueueRequest, do func(ticketRef) (QueueResult, error)) (QueueResult, error) {
	addr, bad := canonical(req.Address)
	if bad != nil {
		return *bad, nil
	}
	ref, ok, err := s.findTicket(addr)
	if err != nil {
		return QueueResult{}, err
	}
	if !ok {
		return refusal(ReasonUnknownTicket, "no ticket "+addr), nil
	}
	return do(ref)
}

// ticketPause pauses a ticket with a person's one-line reason. It refuses a
// running, done, already paused or map-epic ticket.
func (s *Server) ticketPause(req QueueRequest) (QueueResult, error) {
	reason := strings.TrimSpace(req.PauseReason)
	if reason == "" {
		return refusal(ReasonReasonRequired, "a one-line reason is required"), nil
	}
	return s.ticketWrite(req, func(ref ticketRef) (QueueResult, error) {
		if ref.ticket.Status == string(schema.StatusDone) {
			return refusal(ReasonTicketDone, ref.addr.String()+" is done"), nil
		}
		switch schema.Status(ref.ticket.Status) {
		case schema.StatusPaused, schema.StatusLegacyAnswer, schema.StatusLegacyRepair:
			return refusal(ReasonAlreadyPaused, ref.addr.String()+" is already paused"), nil
		}
		if ref.epic.IsMap {
			return refusal(ReasonMapEpic, ref.addr.String()+" is in a map epic; its tickets are resolved by hand, not scheduled"), nil
		}
		run, _ := s.registry.tracked(ref.addr.String())
		if err := s.pauseTicket(ref.projectDir, ref.addr, ref.ticket.Path, events.Manual, reason, run.Run); err != nil {
			return QueueResult{}, fmt.Errorf("pause %s: %w", ref.addr, err)
		}
		return QueueResult{}, nil
	})
}

// ticketUnmute clears a ticket's mutes and reopens it if the mute paused it.
func (s *Server) ticketUnmute(req QueueRequest) (QueueResult, error) {
	return s.ticketWrite(req, func(ref ticketRef) (QueueResult, error) {
		if len(ref.ticket.Mutes) == 0 {
			return refusal(ReasonNotMuted, ref.addr.String()+" is not muted"), nil
		}
		if err := ralphloop.UnmuteTicket(ref.ticket.Path, time.Now()); err != nil {
			return QueueResult{}, fmt.Errorf("unmute %s: %w", ref.addr, err)
		}
		if err := tickets.EndOpenRound(ref.ticket.Path, req.Source, tickets.HowResume, ralphloop.CloseResumed); err != nil {
			return QueueResult{}, fmt.Errorf("end pause round of %s: %w", ref.addr, err)
		}
		s.rescan()
		return QueueResult{}, nil
	})
}

// ticketRelaunch starts a fresh iteration of a ticket outside the queue order,
// under the agent it is queued with (claude otherwise). An idle or done
// leftover pane is left to the launch, which closes a stale label holder. A
// person's relaunch logs human-relaunch, which resets the recovery budget. A
// late finish is adopted instead, with no launch.
func (s *Server) ticketRelaunch(req QueueRequest) (QueueResult, error) {
	return s.ticketWrite(req, func(ref ticketRef) (QueueResult, error) {
		if ref.ticket.Status == string(schema.StatusDone) {
			return refusal(ReasonTicketDone, ref.addr.String()+" is done"), nil
		}
		var applied *recovery.Failure
		if req.investigator != "" {
			f, res, ok := s.investigateRetryFailure(req, ref)
			if !ok {
				return res, nil
			}
			applied = &f
			req.actor = recovery.ActorInvestigate
		}
		if req.State != "" {
			stale, err := ticketPauseStale(ref.ticket.Path, req.State)
			if err != nil {
				return QueueResult{}, err
			}
			if stale {
				return refusal(ReasonStale, "the pause changed; review the current one"), nil
			}
		}
		if s.adoptFirst(ref.projectDir, ref.addr, ref.ticket.Path) {
			return QueueResult{}, nil
		}
		if s.herdr.isUnavailable() {
			return refusal(ReasonHerdrUnavailable, "herdr is unavailable"), nil
		}
		if res, refused := s.budgetRefusal(); refused {
			return res, nil
		}
		if res, refused, err := s.settleLive([]tickets.Address{ref.addr}, req.Stop, ReasonAgentWorking, isWorking); refused || err != nil {
			return res, err
		}
		source := req.Source
		if req.actor != "" {
			source = tickets.SourceGx
		}
		if err := tickets.EndOpenRound(ref.ticket.Path, source, tickets.HowRelaunch, ralphloop.CloseRelaunch); err != nil {
			return QueueResult{}, fmt.Errorf("end pause round of %s: %w", ref.addr, err)
		}
		if req.actor == "" {
			err := ralphloop.AppendEvent(ref.projectDir, ref.addr.Epic, ralphloop.Event{
				Time: time.Now(), Type: string(events.HumanRelaunch), Ticket: ref.addr.ID, Address: ref.addr.String(),
			})
			if err != nil {
				return QueueResult{}, fmt.Errorf("logging relaunch of %s: %w", ref.addr, err)
			}
		}
		epics, err := tickets.LoadResolved(ref.addr.Project, ref.projectDir)
		if err != nil {
			return QueueResult{}, err
		}
		if _, err := s.claimAndLaunch(ref.root(), ref.addr, ref.ticket, ref.repo, ticketAgent(ref.ticket, s.queuedAgent(ref.addr.String())), epics); err != nil {
			return QueueResult{}, err
		}
		if applied != nil {
			s.appendRecoveryEvent(ref, *applied, ralphloop.Event{
				Type: string(events.RecoveryApplied), Ticket: ref.addr.ID, Kind: string(applied.Kind),
				Reason: investigateRetryEntry, Outcome: "ok", Text: req.investigator,
			})
		}
		return QueueResult{}, nil
	})
}

// queuedAgent is the agent the ticket is queued with, claude otherwise.
func (s *Server) queuedAgent(address string) ralphloop.AgentKind {
	agent := ralphloop.AgentClaude
	for _, it := range s.queued.list() {
		if it.Address == address {
			agent = ralphloop.AgentKind(it.Agent)
		}
	}
	return agent
}

// ticketAgent is the agent the ticket's own frontmatter names, fallback when it
// names none or one that is not a known agent.
func ticketAgent(t tickets.Ticket, fallback ralphloop.AgentKind) ralphloop.AgentKind {
	agent := ralphloop.AgentKind(t.Agent)
	if t.Agent == "" || ralphloop.ValidateAgentKind(agent) != nil {
		return fallback
	}
	return agent
}

// ticketCancel withdraws a ticket and every non-terminal ticket forked from it.
// A claimed ticket with a live pane refuses ticket-live unless Stop closes the
// pane first.
func (s *Server) ticketCancel(req QueueRequest) (QueueResult, error) {
	return s.resolvedWrite(req, func(ref ticketRef) (QueueResult, error) {
		if ref.ticket.Status == string(schema.StatusDone) {
			return refusal(ReasonTicketDone, ref.addr.String()+" is done"), nil
		}
		// An archived one-off, found by its old address, is already over.
		if ref.addr.Epic == OneOffsArchivedEpic {
			return refusal(ReasonTicketDone, ref.addr.String()+" is archived"), nil
		}
		targets := cancelTargets(ref)
		addrs := make([]tickets.Address, len(targets))
		for i, t := range targets {
			addrs[i] = tickets.Address{Project: ref.addr.Project, Epic: ref.addr.Epic, ID: t.Identifier}
		}
		anyLive := func(agentrunner.Runner, agentrunner.Session) bool { return true }
		// A one-off is archived on cancel, so its run can't be left alive behind it.
		stop := req.Stop || ref.addr.Epic == OneOffsEpic
		if res, refused, err := s.settleLive(addrs, stop, ReasonTicketLive, anyLive); refused || err != nil {
			return res, err
		}
		for _, t := range targets {
			if err := tickets.EndOpenRound(t.Path, req.Source, tickets.HowCancel, "cancelled"); err != nil {
				return QueueResult{}, fmt.Errorf("end pause round of %s: %w", ref.address(t), err)
			}
			if err := ralphloop.SetStatus(t.Path, string(schema.StatusCancelled)); err != nil {
				return QueueResult{}, fmt.Errorf("cancel %s: %w", ref.address(t), err)
			}
			s.stampFinished(tickets.Address{Project: ref.addr.Project, Epic: ref.addr.Epic, ID: t.Identifier}, t.Path, true)
			s.events.publish(EventTicketCancelled, ref.address(t))
		}
		if ref.addr.Epic == OneOffsEpic {
			s.rescan()
		}
		return QueueResult{}, nil
	})
}

// settleLive is relaunch's and cancel's one view of live agents: liveSession,
// so a leftover pane outside the registry counts too. A live session that
// blocks refuses ticket-live unless stop, which stops every blocking session
// through its runner first.
func (s *Server) settleLive(addrs []tickets.Address, stop bool, reason string, blocks func(agentrunner.Runner, agentrunner.Session) bool) (QueueResult, bool, error) {
	type live struct {
		addr   tickets.Address
		runner agentrunner.Runner
		sess   agentrunner.Session
	}
	var found []live
	for _, a := range addrs {
		r, sess, ok, err := s.liveSession(a)
		if err != nil {
			return QueueResult{}, false, fmt.Errorf("find the live session of %s: %w", a, err)
		}
		if ok && blocks(r, sess) {
			found = append(found, live{a, r, sess})
		}
	}
	if len(found) > 0 && !stop {
		return refusal(reason, found[0].addr.String()+" has a live pane; use --stop"), true, nil
	}
	for _, l := range found {
		if err := l.runner.Stop(l.sess); err != nil {
			return QueueResult{}, false, fmt.Errorf("stop %s: %w", l.addr, err)
		}
	}
	return QueueResult{}, false, nil
}

// isWorking is a session not idle or done; an unknown state counts as working,
// so it is never killed without --stop.
func isWorking(r agentrunner.Runner, sess agentrunner.Session) bool {
	st, err := r.Status(sess)
	return err != nil || !st.Waiting()
}

// cancelTargets is the ticket plus its fork descendants that are not terminal.
func cancelTargets(ref ticketRef) []tickets.Ticket {
	forks := ref.epic.ForkParents()
	targets := []tickets.Ticket{ref.ticket}
	for i := 0; i < len(targets); i++ {
		for _, t := range ref.epic.Tickets {
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

func (r ticketRef) address(t tickets.Ticket) string {
	return tickets.Address{Project: r.addr.Project, Epic: r.addr.Epic, ID: t.Identifier}.String()
}
