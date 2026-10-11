package server

import (
	"fmt"
	"sync"
	"time"

	"github.com/elentok/gx/tickets"
)

const defaultHardLimitGrace = 60 * time.Second

// hardGrace is the countdown a reload starts when it sets the hard limit at or
// below today's spend. The ledger's hold is what pauses starts and keeps the
// hard latch off; the countdown only decides when the hold ends. gen tells a
// timer that fired late from the one running now.
type hardGrace struct {
	mu    sync.Mutex
	timer *time.Timer
	until time.Time
	gen   int
}

// deadline is when the running countdown ends; zero when none runs.
func (g *hardGrace) deadline() time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.until
}

// startHardGrace starts the countdown and announces it, unless one already
// runs: a further lowering reload does not buy more time.
func (s *Server) startHardGrace(now time.Time, limit, spent float64) {
	g := &s.hardGrace
	g.mu.Lock()
	if g.timer != nil {
		g.mu.Unlock()
		return
	}
	grace := s.cfg.HardLimitGrace
	if grace <= 0 {
		grace = defaultHardLimitGrace
	}
	g.gen++
	gen := g.gen
	g.until = now.Add(grace)
	g.timer = time.AfterFunc(grace, func() { s.endHardGrace(gen) })
	g.mu.Unlock()
	s.banner(EventLevelWarning, "gx: agents stop in 60s", fmt.Sprintf(
		"New hard budget limit (%s) is below today's spend (%s). %d agents will stop. Raise the limit in config.json or run `gx budget increase` to cancel.",
		tickets.FormatCost(limit), tickets.FormatCost(spent), len(s.registry.list())))
}

// cancelHardGrace ends a running countdown once a raise has lifted the hold.
func (s *Server) cancelHardGrace() {
	if s.ledger.held() || !s.stopHardGrace() {
		return
	}
	s.kickRunner()
	s.banner(EventLevelSuccess, "gx: agent stop cancelled — hard budget limit raised.", "")
}

// stopHardGrace stops the countdown and reports whether one was running.
func (s *Server) stopHardGrace() bool {
	g := &s.hardGrace
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.timer == nil {
		return false
	}
	g.timer.Stop()
	g.timer, g.until = nil, time.Time{}
	return true
}

// endHardGrace lifts the hold and checks again: spend still at the hard limit
// latches it and stops agents the way spend crossing it does.
func (s *Server) endHardGrace(gen int) {
	g := &s.hardGrace
	g.mu.Lock()
	if g.timer == nil || g.gen != gen {
		g.mu.Unlock()
		return
	}
	g.timer, g.until = nil, time.Time{}
	g.mu.Unlock()

	now := time.Now()
	s.ledger.releaseHold()
	if !s.budgetLatches(now).Hard {
		s.events.publish(EventQueueChanged, "")
		s.kickRunner()
		return
	}
	if err := s.ledger.save(now); err != nil {
		s.log.Error("save budget ledger", "err", err)
	}
	// The banners below are this stop's report; no chat notice follows it.
	s.budgetNotes.markHard()
	s.events.publish(EventQueueChanged, "")
	n := s.killForBudget(now)
	s.banner(EventLevelError, fmt.Sprintf("gx: %d agents stopped — hard budget limit reached.", n), "")
}
