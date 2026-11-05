package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server/disk"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// budgetRefusal refuses new agent work past the daily soft limit; ok is
// false when there is nothing to refuse.
func (s *Server) budgetRefusal() (wire.QueueResult, bool) {
	if !s.budget.SoftReached(time.Now()) {
		return wire.QueueResult{}, false
	}
	return queue.Refusal(wire.ReasonBudgetLimit, "the daily budget limit is reached; raise it with `gx budget override`"), true
}

// FindTicket resolves a canonical address to its ticket, or reports it unknown.
func (s *Server) FindTicket(address string) (queue.TicketRef, bool, error) {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return queue.TicketRef{}, false, err
	}
	projectDir, repo, err := s.projects.Of(addr.Project)
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

// ticketWrite resolves the request's ticket and applies the refusals every
// ticket write shares, before do runs.
func (s *Server) ticketWrite(req wire.QueueRequest, do func(queue.TicketRef) (wire.QueueResult, error)) (wire.QueueResult, error) {
	return s.resolvedWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		if s.registry.Has(ref.Addr.String()) {
			return queue.Refusal(wire.ReasonIterationRunning, "an iteration is running in "+ref.Addr.String()), nil
		}
		return do(ref)
	})
}

// resolvedWrite is ticketWrite without the running-iteration refusal, for
// writes that judge liveness themselves.
func (s *Server) resolvedWrite(req wire.QueueRequest, do func(queue.TicketRef) (wire.QueueResult, error)) (wire.QueueResult, error) {
	addr, bad := queue.Canonical(req.Address)
	if bad != nil {
		return *bad, nil
	}
	ref, ok, err := s.FindTicket(addr)
	if err != nil {
		return wire.QueueResult{}, err
	}
	if !ok {
		return queue.Refusal(wire.ReasonUnknownTicket, "no ticket "+addr), nil
	}
	return do(ref)
}

// ticketPause pauses a ticket with a person's one-line reason. It refuses a
// running, done, already paused or map-epic ticket.
func (s *Server) ticketPause(req wire.QueueRequest) (wire.QueueResult, error) {
	reason := strings.TrimSpace(req.PauseReason)
	if reason == "" {
		return queue.Refusal(wire.ReasonReasonRequired, "a one-line reason is required"), nil
	}
	return s.ticketWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		if ref.Ticket.Status == string(schema.StatusDone) {
			return queue.Refusal(wire.ReasonTicketDone, ref.Addr.String()+" is done"), nil
		}
		switch schema.Status(ref.Ticket.Status) {
		case schema.StatusPaused, schema.StatusLegacyAnswer, schema.StatusLegacyRepair:
			return queue.Refusal(wire.ReasonAlreadyPaused, ref.Addr.String()+" is already paused"), nil
		}
		if ref.Epic.IsMap {
			return queue.Refusal(wire.ReasonMapEpic, ref.Addr.String()+" is in a map epic; its tickets are resolved by hand, not scheduled"), nil
		}
		run, _ := s.registry.Tracked(ref.Addr.String())
		if err := s.pauseTicket(ref.ProjectDir, ref.Addr, ref.Ticket.Path, events.Manual, reason, run.Run); err != nil {
			return wire.QueueResult{}, fmt.Errorf("pause %s: %w", ref.Addr, err)
		}
		return wire.QueueResult{}, nil
	})
}

// ticketUnmute clears a ticket's mutes and reopens it if the mute paused it.
func (s *Server) ticketUnmute(req wire.QueueRequest) (wire.QueueResult, error) {
	return s.ticketWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		if len(ref.Ticket.Mutes) == 0 {
			return queue.Refusal(wire.ReasonNotMuted, ref.Addr.String()+" is not muted"), nil
		}
		if err := ralphloop.UnmuteTicket(ref.Ticket.Path, time.Now()); err != nil {
			return wire.QueueResult{}, fmt.Errorf("unmute %s: %w", ref.Addr, err)
		}
		if err := tickets.EndOpenRound(ref.Ticket.Path, req.Source, tickets.HowResume, ralphloop.CloseResumed); err != nil {
			return wire.QueueResult{}, fmt.Errorf("end pause round of %s: %w", ref.Addr, err)
		}
		s.idx.Rescan()
		return wire.QueueResult{}, nil
	})
}

// retryWrite serves the retry route. Retry on an answer-class pause whose
// agent is still running prompts that agent through the resume dispatch (status
// prompted); every other retry relaunches. An investigator's retry always
// relaunches.
func (s *Server) retryWrite(web bool) http.HandlerFunc {
	relaunch := queue.HandlerAs(web, s.ticketRelaunch)
	resume := repairWriteAs(web, s.repairResume)
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		var req struct {
			Address string `json:"address"`
		}
		if json.Unmarshal(body, &req) == nil && (web || r.Header.Get(wire.InvestigateHeader) == "") && s.promptsOnRetry(req.Address) {
			resume(w, r)
			return
		}
		relaunch(w, r)
	}
}

// promptsOnRetry reports whether a retry of address prompts its live agent
// instead of relaunching.
func (s *Server) promptsOnRetry(address string) bool {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return false
	}
	_, t, ok := s.FreshTicket(addr)
	if !ok {
		return false
	}
	tk, err := schema.ParseTicket(t.Path)
	if err != nil || !tk.PausedAsking() {
		return false
	}
	if _, ok := retryPrompts[tk.PauseReason]; !ok {
		return false
	}
	dir, err := s.projects.Dir(addr.Project)
	if err != nil {
		return false
	}
	_, _, live, err := s.iterationSession(dir, addr)
	return err == nil && live
}

// ticketRelaunch starts a fresh iteration of a ticket outside the queue order,
// under the harness the resolver picks for it. An idle or done
// leftover pane is left to the launch, which closes a stale label holder. A
// person's relaunch logs human-relaunch, which resets the recovery budget. A
// late finish is adopted instead, with no launch.
func (s *Server) ticketRelaunch(req wire.QueueRequest, qc queue.Ctx) (wire.QueueResult, error) {
	return s.ticketWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		if ref.Ticket.Status == string(schema.StatusDone) {
			return queue.Refusal(wire.ReasonTicketDone, ref.Addr.String()+" is done"), nil
		}
		var applied *recovery.Failure
		if qc.Investigator != "" {
			f, res, ok := s.investigateRetryFailure(qc, ref)
			if !ok {
				return res, nil
			}
			applied = &f
			qc.Actor = recovery.ActorInvestigate
		}
		if req.State != "" {
			stale, err := ticketPauseStale(ref.Ticket.Path, req.State)
			if err != nil {
				return wire.QueueResult{}, err
			}
			if stale {
				return queue.Refusal(wire.ReasonStale, "the pause changed; review the current one"), nil
			}
		}
		if s.adoptFirst(ref.ProjectDir, ref.Addr, ref.Ticket.Path) {
			return wire.QueueResult{}, nil
		}
		agent := s.harness.ResolveTicket(ref.Addr, ref.Ticket).Harness
		if reason, held := s.herdrHolds(s.harness.LaunchRunner(ref.Addr.Project, agent)); held {
			return queue.Refusal(wire.ReasonHerdrUnavailable, reason), nil
		}
		if res, refused := s.budgetRefusal(); refused {
			return res, nil
		}
		if qc.Actor != "" {
			if v, held := s.limits.UsageGate(agent, time.Now()); held {
				return queue.Refusal(wire.ReasonUsageLimit, v), nil
			}
		}
		if d, held := s.disks.FreshHeldDisk(ref.Addr.Project); held {
			return queue.Refusal(wire.ReasonDiskLow, disk.LowReason(d)), nil
		}
		if res, refused, err := s.settleLive([]tickets.Address{ref.Addr}, req.Stop, wire.ReasonAgentWorking, isWorking); refused || err != nil {
			return res, err
		}
		source := req.Source
		if qc.Actor != "" {
			source = tickets.SourceGx
		}
		if err := tickets.EndOpenRound(ref.Ticket.Path, source, tickets.HowRelaunch, ralphloop.CloseRelaunch); err != nil {
			return wire.QueueResult{}, fmt.Errorf("end pause round of %s: %w", ref.Addr, err)
		}
		if qc.Actor == "" {
			if err := s.cancelInvestigateChild(ref, req.Source); err != nil {
				return wire.QueueResult{}, err
			}
			err := ralphloop.AppendEvent(ref.ProjectDir, ref.Addr.Epic, ralphloop.Event{
				Time: time.Now(), Type: string(events.HumanRelaunch), Ticket: ref.Addr.ID, Address: ref.Addr.String(),
			})
			if err != nil {
				return wire.QueueResult{}, fmt.Errorf("logging relaunch of %s: %w", ref.Addr, err)
			}
		}
		epics, err := tickets.LoadResolved(ref.Addr.Project, ref.ProjectDir)
		if err != nil {
			return wire.QueueResult{}, err
		}
		if _, err := s.claimAndLaunch(ref.Root(), ref.Addr, ref.Ticket, ref.Repo, agent, epics); err != nil {
			return wire.QueueResult{}, err
		}
		if applied != nil {
			s.appendRecoveryEvent(ref, *applied, ralphloop.Event{
				Type: string(events.RecoveryApplied), Ticket: ref.Addr.ID, Kind: string(applied.Kind),
				Reason: investigateRetryEntry, Outcome: "ok", Text: qc.Investigator,
			})
		}
		return wire.QueueResult{}, nil
	})
}

// ticketCancel withdraws a ticket and every non-terminal ticket forked from it.
// A claimed ticket with a live pane refuses ticket-live unless Stop closes the
// pane first.
func (s *Server) ticketCancel(req wire.QueueRequest) (wire.QueueResult, error) {
	return s.resolvedWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		if ref.Ticket.Status == string(schema.StatusDone) {
			return queue.Refusal(wire.ReasonTicketDone, ref.Addr.String()+" is done"), nil
		}
		// An archived one-off, found by its old address, is already over.
		if ref.Addr.Epic == queue.OneOffsArchivedEpic {
			return queue.Refusal(wire.ReasonTicketDone, ref.Addr.String()+" is archived"), nil
		}
		targets := cancelTargets(ref)
		addrs := make([]tickets.Address, len(targets))
		for i, t := range targets {
			addrs[i] = tickets.Address{Project: ref.Addr.Project, Epic: ref.Addr.Epic, ID: t.Identifier}
		}
		anyLive := func(agentrunner.Runner, agentrunner.Session) bool { return true }
		// A one-off is archived on cancel, so its run can't be left alive behind it.
		stop := req.Stop || ref.Addr.Epic == queue.OneOffsEpic
		if res, refused, err := s.settleLive(addrs, stop, wire.ReasonTicketLive, anyLive); refused || err != nil {
			return res, err
		}
		for _, t := range targets {
			if err := s.markCancelled(ref, t, req.Source); err != nil {
				return wire.QueueResult{}, err
			}
		}
		if ref.Addr.Epic == queue.OneOffsEpic {
			s.idx.Rescan()
		}
		return wire.QueueResult{}, nil
	})
}

// markCancelled closes t's pause round and settles it as cancelled. Its live
// agent is the caller's to stop.
func (s *Server) markCancelled(ref queue.TicketRef, t tickets.Ticket, source string) error {
	if err := tickets.EndOpenRound(t.Path, source, tickets.HowCancel, "cancelled"); err != nil {
		return fmt.Errorf("end pause round of %s: %w", ref.Address(t), err)
	}
	if err := ralphloop.SetStatus(t.Path, string(schema.StatusCancelled)); err != nil {
		return fmt.Errorf("cancel %s: %w", ref.Address(t), err)
	}
	s.stampFinished(tickets.Address{Project: ref.Addr.Project, Epic: ref.Addr.Epic, ID: t.Identifier}, t.Path, true)
	// The branch stays: it holds the commits.
	if ref.Repo != "" && s.awaitRunSettled(ref.Address(t)) {
		// The epic merge removes leftovers later, so a failed removal must not stop the cancel.
		if err := ralphloop.RemoveIterationWorktree(s.projects.DefaultDeps(), ref.Repo, ref.Addr.Epic, t.Identifier); err != nil {
			s.log.Warn("remove cancelled worktree", "ticket", ref.Address(t), "err", err)
		}
	}
	s.events.Publish(wire.EventTicketCancelled, ref.Address(t))
	return nil
}

// awaitRunSettled waits for a stopped run to leave the registry: its finish
// still reads the worktree, and a missing one would pause the cancelled ticket.
// False means the run never settled, so its worktree is left for the epic merge.
func (s *Server) awaitRunSettled(addr string) bool {
	return s.registry.WaitGone(addr, 5*time.Second, 20*time.Millisecond)
}

// cancelInvestigateChild is a person's recovery verb withdrawing ref's
// unfinished investigate child, stopping its live agent, so an old investigation
// does not act after the person. Investigate's own verbs never call it.
func (s *Server) cancelInvestigateChild(ref queue.TicketRef, source string) error {
	forks := ref.Epic.ForkParents()
	for _, t := range ref.Epic.Tickets {
		parent, ok := forks.Of(t)
		if !ok || parent.Identifier != ref.Ticket.Identifier || t.Type != string(schema.TypeInvestigate) || isTerminal(t.Status) {
			continue
		}
		addr := tickets.Address{Project: ref.Addr.Project, Epic: ref.Addr.Epic, ID: t.Identifier}
		anyLive := func(agentrunner.Runner, agentrunner.Session) bool { return true }
		if _, _, err := s.settleLive([]tickets.Address{addr}, true, wire.ReasonTicketLive, anyLive); err != nil {
			return err
		}
		if err := s.markCancelled(ref, t, source); err != nil {
			return err
		}
	}
	return nil
}

// cancelInvestigateOf is cancelInvestigateChild for a ticket named by address.
func (s *Server) cancelInvestigateOf(address, source string) error {
	ref, ok, err := s.FindTicket(address)
	if err != nil || !ok {
		return err
	}
	return s.cancelInvestigateChild(ref, source)
}

// settleLive is relaunch's and cancel's one view of live agents: liveSession,
// so a leftover pane outside the registry counts too. A live session that
// blocks refuses ticket-live unless stop, which stops every blocking session
// through its runner first.
func (s *Server) settleLive(addrs []tickets.Address, stop bool, reason string, blocks func(agentrunner.Runner, agentrunner.Session) bool) (wire.QueueResult, bool, error) {
	type live struct {
		addr   tickets.Address
		runner agentrunner.Runner
		sess   agentrunner.Session
	}
	var found []live
	for _, a := range addrs {
		r, sess, ok, err := s.harness.LiveSession(a)
		if err != nil {
			return wire.QueueResult{}, false, fmt.Errorf("find the live session of %s: %w", a, err)
		}
		if ok && blocks(r, sess) {
			found = append(found, live{a, r, sess})
		}
	}
	if len(found) > 0 && !stop {
		return queue.Refusal(reason, found[0].addr.String()+" has a live pane; use --stop"), true, nil
	}
	for _, l := range found {
		if err := l.runner.Stop(l.sess); err != nil {
			return wire.QueueResult{}, false, fmt.Errorf("stop %s: %w", l.addr, err)
		}
	}
	return wire.QueueResult{}, false, nil
}

// isWorking is a session not idle or done; an unknown state counts as working,
// so it is never killed without --stop.
func isWorking(r agentrunner.Runner, sess agentrunner.Session) bool {
	st, err := r.Status(sess)
	return err != nil || !st.Waiting()
}

// cancelTargets is the ticket plus its fork descendants that are not terminal.
func cancelTargets(ref queue.TicketRef) []tickets.Ticket {
	forks := ref.Epic.ForkParents()
	targets := []tickets.Ticket{ref.Ticket}
	for i := 0; i < len(targets); i++ {
		for _, t := range ref.Epic.Tickets {
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
