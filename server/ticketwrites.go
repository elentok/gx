package server

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
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
}

func (r ticketRef) root() string { return r.addr.Project + ":" + r.addr.Epic }

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
				return ticketRef{addr: addr, projectDir: projectDir, repo: repo, ticket: t}, true, nil
			}
		}
	}
	return ticketRef{}, false, nil
}

// ticketWrite resolves the request's ticket and applies the refusals every
// ticket write shares, before do runs.
func (s *Server) ticketWrite(req QueueRequest, do func(ticketRef) (QueueResult, error)) (QueueResult, error) {
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
	if s.registry.has(ref.root()) {
		return refusal(ReasonIterationRunning, "an iteration is running in "+ref.root()), nil
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
		if err := ralphloop.ParkNeedsRepair(ref.projectDir, ref.addr.Epic, ref.addr.ID, ref.ticket.Path, reason); err != nil {
			return QueueResult{}, fmt.Errorf("park %s: %w", ref.addr, err)
		}
		s.events.publish(EventTicketParked, ref.addr.String())
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
		agent := ralphloop.AgentClaude
		for _, it := range s.queued.list() {
			if it.Address == ref.addr.String() {
				agent = ralphloop.AgentKind(it.Agent)
			}
		}
		if err := s.claimAndLaunch(ref.root(), ref.addr, ref.ticket, ref.repo, agent); err != nil {
			return QueueResult{}, err
		}
		return QueueResult{}, nil
	})
}
