package server

import (
	"fmt"
	"github.com/elentok/gx/events"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// Refusal reasons of the ticket writes (park, relaunch).
const (
	ReasonReasonRequired   = "reason-required"
	ReasonIterationRunning = "iteration-running"
	ReasonTicketDone       = "ticket-done"
	ReasonHerdrUnavailable = "herdr-unavailable"
)

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
	epics, err := tickets.Load(projectDir)
	if err != nil {
		return ticketRef{}, false, err
	}
	for _, e := range epics {
		if filepath.Base(e.Path) != addr.Epic {
			continue
		}
		for _, t := range e.Tickets {
			if t.Identifier == addr.ID {
				return ticketRef{addr: addr, projectDir: projectDir, repo: repo, ticket: t, epic: e}, true, nil
			}
		}
	}
	return ticketRef{}, false, nil
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
	if s.cfg.Orchestrator != config.OrchestratorServer {
		return refusal(ReasonSchedulerNotSelected, `orchestrator is not "server"`), nil
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

// ticketPark parks a ticket needs-repair with a person's one-line reason.
func (s *Server) ticketPark(req QueueRequest) (QueueResult, error) {
	reason := strings.TrimSpace(req.ParkReason)
	if reason == "" {
		return refusal(ReasonReasonRequired, "a one-line reason is required"), nil
	}
	return s.ticketWrite(req, func(ref ticketRef) (QueueResult, error) {
		if ref.ticket.Status == string(schema.StatusDone) {
			return refusal(ReasonTicketDone, ref.addr.String()+" is done"), nil
		}
		if err := ralphloop.Park(ref.projectDir, ref.addr.Epic, ref.addr.ID, ref.ticket.Path, events.ManualPark, reason); err != nil {
			return QueueResult{}, fmt.Errorf("park %s: %w", ref.addr, err)
		}
		s.events.publish(EventTicketParked, ref.addr.String())
		if req.actor != recovery.ActorRecovery {
			s.recoverAsync(recovery.Failure{Address: ref.addr.String(), Type: events.ManualPark.ParkType(), Kind: events.ManualPark, Reason: reason})
		}
		return QueueResult{}, nil
	})
}

// ticketRelaunch starts a fresh iteration of a ticket outside the queue order,
// under the agent it is queued with (claude otherwise).
func (s *Server) ticketRelaunch(req QueueRequest) (QueueResult, error) {
	return s.ticketWrite(req, func(ref ticketRef) (QueueResult, error) {
		if ref.ticket.Status == string(schema.StatusDone) {
			return refusal(ReasonTicketDone, ref.addr.String()+" is done"), nil
		}
		if s.herdr.isUnavailable() {
			return refusal(ReasonHerdrUnavailable, "herdr is unavailable"), nil
		}
		epics, err := tickets.Load(ref.projectDir)
		if err != nil {
			return QueueResult{}, err
		}
		if _, err := s.claimAndLaunch(ref.root(), ref.addr, ref.ticket, ref.repo, s.queuedAgent(ref.addr.String()), epics); err != nil {
			return QueueResult{}, err
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

// ticketCancel withdraws a ticket and every non-terminal ticket forked from it.
// A claimed ticket with a live pane refuses ticket-live unless Stop closes the
// pane first.
func (s *Server) ticketCancel(req QueueRequest) (QueueResult, error) {
	return s.resolvedWrite(req, func(ref ticketRef) (QueueResult, error) {
		if ref.ticket.Status == string(schema.StatusDone) {
			return refusal(ReasonTicketDone, ref.addr.String()+" is done"), nil
		}
		targets := cancelTargets(ref)
		var live []Run
		for _, t := range targets {
			if run, ok := s.registry.runOf(ref.address(t)); ok {
				live = append(live, run)
			}
		}
		if len(live) > 0 && !req.Stop {
			return refusal(ReasonTicketLive, live[0].Address+" has a live pane; use --stop"), nil
		}
		for _, run := range live {
			if err := herdr.TabClose(run.Tab); err != nil {
				return QueueResult{}, fmt.Errorf("stop %s: %w", run.Address, err)
			}
		}
		for _, t := range targets {
			if err := ralphloop.SetStatus(t.Path, string(schema.StatusCancelled)); err != nil {
				return QueueResult{}, fmt.Errorf("cancel %s: %w", ref.address(t), err)
			}
			s.events.publish(EventTicketCancelled, ref.address(t))
		}
		return QueueResult{}, nil
	})
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
