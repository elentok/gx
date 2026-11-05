package server

import (
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/tickets"
)

// LogEvent appends ev to ticket's run log. It is the root's merge.Host method.
func (s *Server) LogEvent(ticket tickets.Address, ev runlog.Event) {
	s.launcher.LogEvent(ticket, ev)
}

// RootRefs is the root's merge.Host method; the base rules live in sched.
func (s *Server) RootRefs(project string, epics []tickets.Epic, epic string, t tickets.Ticket, repo string) (base, target string, err error) {
	return s.scheduler.RootRefs(project, epics, epic, t, repo)
}

// MergeTarget is the root's merge.Host method; the base rules live in sched.
func (s *Server) MergeTarget(addr tickets.Address) (string, error) {
	return s.scheduler.MergeTarget(addr)
}
