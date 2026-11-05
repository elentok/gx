package server

import (
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/runnerbuild"
	"github.com/elentok/gx/tickets"
)

// herdrHost is sched.HerdrHost.
type herdrHost struct{ s *Server }

// Runner is the runner that hosts herdr tickets, or nil when the server cannot
// build one.
func (h herdrHost) Runner() agentrunner.Runner {
	return h.s.harness.RunnerNamed(runs.RunnerHerdr, runnerbuild.FallbackProject)
}

func (h herdrHost) Configured() bool { return h.s.cfg.Runner.Name() == runs.RunnerHerdr }

func (h herdrHost) Publish(event, id string) { h.s.events.Publish(event, id) }

func (h herdrHost) Notice(n ralphloop.ServerNotice) { h.s.chat.Notice(n) }

func (h herdrHost) FlushPauseFold() { h.s.writer.FlushPauseFold() }

// Waiting is the tickets that need herdr: queued ones that launch on it and
// tracked runs it hosts. A queued root waits when its next ticket, with the
// agent the claim picks for it, launches on herdr.
func (h herdrHost) Waiting() []string {
	s := h.s
	var out []string
	seen := map[string]bool{}
	for _, item := range s.queue.List() {
		addr, err := tickets.ParseQueuedAddress(item.Address)
		if err != nil || seen[queue.RootOf(addr).String()] {
			continue
		}
		seen[queue.RootOf(addr).String()] = true
		t, ok := s.explain.NextQueuedTicket(addr, s.projects.Epics)
		if !ok {
			continue
		}
		agent := s.harness.ResolveTicket(tickets.Address{Project: addr.Project, Epic: addr.Epic, ID: t.Identifier}, t).Harness
		if r := s.harness.LaunchRunner(addr.Project, agent); r != nil && r.Name() == runs.RunnerHerdr {
			out = append(out, item.Address)
		}
	}
	for _, t := range s.registry.List() {
		if r := s.harness.RunnerForRun(t.Run); r != nil && r.Name() == runs.RunnerHerdr {
			out = append(out, t.Address)
		}
	}
	return out
}
