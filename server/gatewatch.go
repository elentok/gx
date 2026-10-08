package server

import (
	"context"
	"strings"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/tickets"
)

const (
	// gateWatchInterval is slow on purpose: each pass reads every live run's
	// log, and the server must stay cheap while idle.
	gateWatchInterval = time.Minute
	// gateHeldQuietPeriod is how long a hold stands before it counts as stuck.
	// Well under the loop's own aged-out cap, so the raise comes first.
	gateHeldQuietPeriod = 15 * time.Minute
)

// gateHold is one held-gate event, raised at most once per server lifetime.
// The per-kind guard rail stops a restart from recovering it twice.
type gateHold struct {
	address string
	at      time.Time
}

// keepGatesWatched raises held background-task gates until ctx ends. The loop
// only logs a hold and keeps polling; nothing else turns it into a failure.
func (s *Server) keepGatesWatched(ctx context.Context) {
	ticker := time.NewTicker(gateWatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.watchGates(time.Now())
		}
	}
}

// watchGates raises each live run whose latest iteration holds a background
// task gate past the quiet period while its pane is idle. R10 ships disabled,
// so for now the raise matches nothing and goes to an investigation.
func (s *Server) watchGates(now time.Time) {
	if s.gatesRaised == nil {
		s.gatesRaised = map[gateHold]bool{}
	}
	for _, run := range s.registry.list() {
		addr, err := tickets.ParseAddress(run.Address, tickets.AddressContext{})
		if err != nil {
			continue
		}
		held, ok := s.heldGate(addr)
		if !ok || now.Sub(held.Time) < gateHeldQuietPeriod {
			continue
		}
		hold := gateHold{address: run.Address, at: held.Time}
		if s.gatesRaised[hold] {
			continue
		}
		label, _, _ := ralphloop.IterationIdentity(addr.Epic, addr.ID, "")
		agent, err := herdr.AgentGet(label)
		if err != nil || (agent.AgentStatus != "idle" && agent.AgentStatus != "done") {
			continue
		}
		s.gatesRaised[hold] = true
		s.recoverAsync(recovery.Failure{Address: run.Address, Type: events.BackgroundTaskGateHeld, Kind: events.BackgroundTaskGate, Reason: held.Reason})
	}
}

// heldGate is the newest gate-held event of the ticket's latest iteration
// whose task is still held.
func (s *Server) heldGate(addr tickets.Address) (ralphloop.Event, bool) {
	dir, _, err := s.projectOf(addr.Project)
	if err != nil {
		return ralphloop.Event{}, false
	}
	log, _, err := ralphloop.ReadEvents(dir, addr.Epic)
	if err != nil {
		s.log.Warn("gate watch cannot read the run log", "ticket", addr, "err", err)
		return ralphloop.Event{}, false
	}
	held := map[string]ralphloop.Event{}
	for _, ev := range log {
		if ev.Ticket != addr.ID {
			continue
		}
		switch events.Type(ev.Type) {
		case events.IterationStarted:
			clear(held)
		case events.BackgroundTaskGateHeld:
			held[gateTask(ev.Reason)] = ev
		case events.BackgroundTaskGateReleased, events.BackgroundTaskGateExpired:
			delete(held, gateTask(ev.Reason))
		}
	}
	var newest ralphloop.Event
	for _, ev := range held {
		if ev.Time.After(newest.Time) {
			newest = ev
		}
	}
	return newest, len(held) > 0
}

// gateTask is the task ID a gate event's reason names, e.g. "background task
// X: recovery forced the release" → "X".
func gateTask(reason string) string {
	id := strings.TrimPrefix(reason, "background task ")
	id, _, _ = strings.Cut(id, ":")
	return id
}
