package server

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/runnerbuild"
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
	// missing is herdr not on PATH. It holds herdr tickets, but only reports
	// herdr unavailable when herdr is the configured runner: a person running
	// PTY without herdr installed has nothing to be told.
	missing bool
}

func (w *herdrWatch) isUnavailable() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.unavailable
}

// holdReason says why herdr tickets wait, or reports that they need not.
func (w *herdrWatch) holdReason() (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	switch {
	case w.missing:
		return "herdr was not found on PATH", true
	case w.unavailable:
		return "herdr is down", true
	}
	return "", false
}

// probe records r's host health and reports whether unavailable changed. A
// runner with no host never reports herdr down. configured is whether herdr is
// the configured runner.
func (w *herdrWatch) probe(r agentrunner.Runner, configured bool) (changed bool) {
	err := agentrunner.Healthy(r)
	missing := errors.Is(err, exec.ErrNotFound)
	down := err != nil && (!missing || configured)
	w.mu.Lock()
	defer w.mu.Unlock()
	changed = down != w.unavailable
	w.unavailable, w.missing = down, missing
	return changed
}

// herdrRunner is the runner that hosts herdr tickets, or nil when the server
// cannot build one.
func (s *Server) herdrRunner() agentrunner.Runner {
	return s.runnerNamed(runnerHerdr, runnerbuild.FallbackProject)
}

// checkHerdr probes once and, on a transition, logs and streams it.
func (s *Server) checkHerdr() {
	if !s.herdr.probe(s.herdrRunner(), s.cfg.Runner.Name() == runnerHerdr) {
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
	s.flushPauseFold()
}

// keepHerdrChecked retries herdr until ctx ends, so a herdr that starts late
// (or dies later) is noticed without a server restart.
func (s *Server) keepHerdrChecked(ctx context.Context) {
	if _, ok := s.herdrRunner().(agentrunner.HealthChecker); !ok {
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
