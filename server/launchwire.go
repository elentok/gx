package server

import (
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// launchHost is sched.LaunchHost.
type launchHost struct{ s *Server }

func (h launchHost) ResolveBase(addr tickets.Address, t tickets.Ticket, repo string, epics []tickets.Epic) (ref, stamp, leaf string, err error) {
	return h.s.resolveBase(addr, t, repo, epics)
}

func (h launchHost) CommitlessRef(addr tickets.Address, t tickets.Ticket, repo string, epics []tickets.Epic) (string, error) {
	return h.s.commitlessRef(addr, t, repo, epics)
}

func (h launchHost) MergeTargetTip(addr tickets.Address, repo string) (string, error) {
	return h.s.mergeTargetTip(addr, repo)
}

func (h launchHost) StalledNudgeDelay() time.Duration { return h.s.stalledNudgeDelay() }

// LogEvent appends ev to ticket's run log. It is the root's merge.Host method.
func (s *Server) LogEvent(ticket tickets.Address, ev ralphloop.Event) {
	s.launcher.LogEvent(ticket, ev)
}
