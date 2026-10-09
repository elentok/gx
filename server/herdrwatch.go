package server

import (
	"context"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
)

const (
	// EventHerdrUnavailable and EventHerdrAvailable are emitted on transition
	// only, never per failed retry.
	EventHerdrUnavailable = "herdr-unavailable"
	EventHerdrAvailable   = "herdr-available"

	defaultHerdrRetryInterval = 30 * time.Second
)

// herdrWatch tracks whether herdr answers. The server starts either way; this
// only reports the state and the transitions.
type herdrWatch struct {
	mu          sync.Mutex
	unavailable bool
}

func (w *herdrWatch) isUnavailable() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.unavailable
}

// probe records r's host health and reports whether it changed. A runner
// with no host never reports herdr down.
func (w *herdrWatch) probe(r agentrunner.Runner) (changed bool) {
	down := agentrunner.Healthy(r) != nil
	w.mu.Lock()
	defer w.mu.Unlock()
	changed = down != w.unavailable
	w.unavailable = down
	return changed
}

// checkHerdr probes once and, on a transition, logs and streams it.
func (s *Server) checkHerdr() {
	if !s.herdr.probe(s.cfg.Runner) {
		return
	}
	typ := EventHerdrAvailable
	if s.herdr.isUnavailable() {
		typ = EventHerdrUnavailable
	}
	s.log.Warn("herdr state changed", "event", typ)
	s.events.publish(typ, "")
	if typ == EventHerdrUnavailable {
		s.chat.Notice(ralphloop.ServerNotice{Kind: typ, Emoji: "🔌", Title: "herdr unavailable", Detail: "Agents cannot start until herdr answers again"})
		return
	}
	s.chat.Notice(ralphloop.ServerNotice{Kind: typ, Emoji: "✅", Title: "herdr back", Detail: "herdr answers again"})
	s.flushParkFold()
}

// keepHerdrChecked retries herdr until ctx ends, so a herdr that starts late
// (or dies later) is noticed without a server restart.
func (s *Server) keepHerdrChecked(ctx context.Context) {
	if _, ok := s.cfg.Runner.(agentrunner.HealthChecker); !ok {
		return
	}
	interval := s.cfg.HerdrRetryInterval
	if interval <= 0 {
		interval = defaultHerdrRetryInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkHerdr()
		}
	}
}
