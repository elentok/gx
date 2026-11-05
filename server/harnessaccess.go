package server

import (
	"path/filepath"
	"time"

	"github.com/elentok/gx/server/budget"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/limits"
	"github.com/elentok/gx/server/queue"
	srvrecovery "github.com/elentok/gx/server/recovery"
	"github.com/elentok/gx/server/webauth"
	"github.com/elentok/gx/tickets"
)

// queueHost is queue.Host: the harness methods the queue reads, and the rest
// from the root.
type queueHost struct {
	sessionHost
	*harnesschoice.Harness
}

// CheckExtraUsage forwards to the budget, which owns the daily notice latch.
func (h queueHost) CheckExtraUsage(now time.Time) bool { return h.budget.CheckExtraUsage(now) }

// StampEpic forwards to the merger, which owns the epic stamps.
func (h queueHost) StampEpic(root queue.RootRef, epicPath string, keys ...string) {
	h.merger.StampEpic(root, epicPath, keys...)
}

// Harness is the server's harness choice. Tests outside the package read it
// through here.
func (s *Server) Harness() *harnesschoice.Harness { return s.harness }

// Limits is the usage limits and token poll. Tests outside the package read
// their seams through here.
func (s *Server) Limits() *limits.Limits { return s.limits }

// Budget is the daily spend budget. Tests outside the package read its seams
// through here.
func (s *Server) Budget() *budget.Budget { return s.budget }

// Recovery is the failure-recovery subsystem. Tests outside the package read
// its seams through here.
func (s *Server) Recovery() *srvrecovery.Recovery { return s.recovery }

// SettleReleasedRun stands in for the finish of a run whose gate was released:
// the pause release-gate left, if any, then the run is dropped. For tests.
func (s *Server) SettleReleasedRun(address string) {
	addr, _ := tickets.ParseAddress(address, tickets.AddressContext{})
	dir := filepath.Join(s.cfg.TicketStore, addr.Project)
	if run, ok := s.registry.RunOf(address); ok {
		s.recovery.PauseReleasedGate(dir, addr, filepath.Join(dir, addr.Epic, "issues", addr.ID+"-first.md"), run)
	}
	s.registry.Delete(address)
}

// AgentWaiting reports whether the run's agent is not working (idle or done).
// For tests.
func (s *Server) AgentWaiting(project, epic, id string) bool {
	_, idle, err := s.recovery.AgentState(tickets.Address{Project: project, Epic: epic, ID: id})
	return err == nil && idle
}

// WatchGates runs one gate watchdog pass as of now. For tests.
func (s *Server) WatchGates(now time.Time) { s.watchGates(now) }

// WebAuth is the web login gate, nil unless the web listener is bound. Tests
// outside the package read its seams through here.
func (s *Server) WebAuth() *webauth.Gate { return s.web.Gate() }
