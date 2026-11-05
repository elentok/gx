package server

import (
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/tickets"
)

// newTicketWriter needs s.log, idx, events, projects and chat set.
func newTicketWriter(s *Server) *ticketwrite.Writer {
	return ticketwrite.New(ticketwrite.Deps{
		Log: s.log, Index: s.idx, Events: s.events, Projects: s.projects, Chat: s.chat,
		Host: ticketWriteHost{s},
	})
}

// ticketWriteHost is ticketwrite.Host.
type ticketWriteHost struct{ s *Server }

func (h ticketWriteHost) HoldPauseForRecovery(addr tickets.Address, ticketPath string, kind events.Kind, reason string) bool {
	return h.s.holdPauseForRecovery(addr, ticketPath, kind, reason)
}

func (h ticketWriteHost) WriteFollowUp(from tickets.Address, f ticketwrite.FollowUp) (string, error) {
	return h.s.writeFollowUp(from, followUp(f))
}

func (h ticketWriteHost) FindTicket(address string) (string, string, bool, error) {
	ref, ok, err := h.s.FindTicket(address)
	return ref.ProjectDir, ref.Ticket.Path, ok, err
}

func (h ticketWriteHost) HerdrUnavailable() bool { return h.s.herdr.isUnavailable() }

func (h ticketWriteHost) StopRun(run runs.Run) error {
	r, err := h.s.harness.RunnerOf(run)
	if err != nil {
		return err
	}
	return r.Stop(run.Session)
}

func (h ticketWriteHost) TakeTierOverride(ticketPath string) ralphloop.Tier {
	return takeTierOverride(ticketPath)
}
