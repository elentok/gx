package server

import (
	"context"
	"errors"
	"os/exec"
	"strings"
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
	// shown is the banner state last streamed: herdr down while a ticket needs it.
	shown bool
}

// setShown records the banner state and reports whether it changed.
func (w *herdrWatch) setShown(v bool) (changed bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	changed = v != w.shown
	w.shown = v
	return changed
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

// herdrWaiting is the tickets that need herdr: queued ones that launch on it
// and tracked runs it hosts. A queued root waits when its next ticket, with the
// agent the claim picks for it, launches on herdr.
func (s *Server) herdrWaiting() []string {
	var out []string
	seen := map[string]bool{}
	for _, item := range s.queued.list() {
		addr, err := parseQueuedAddress(item.Address)
		if err != nil || seen[rootOf(addr).String()] {
			continue
		}
		seen[rootOf(addr).String()] = true
		t, ok := s.nextQueuedTicket(addr, s.projectEpics)
		if !ok {
			continue
		}
		agent := ticketAgent(t, s.queuedAgent(item.Address))
		if r := s.launchRunner(addr.Project, agent); r != nil && r.Name() == runnerHerdr {
			out = append(out, item.Address)
		}
	}
	for _, t := range s.registry.list() {
		if r := s.runnerForRun(t.Run); r != nil && r.Name() == runnerHerdr {
			out = append(out, t.Address)
		}
	}
	return out
}

// herdrBanner is the text of the "herdr down" banner, and whether it shows: herdr
// is down or missing and a ticket needs it.
func (s *Server) herdrBanner() (string, bool) {
	reason, down := s.herdr.holdReason()
	if !down {
		return "", false
	}
	waiting := s.herdrWaiting()
	if len(waiting) == 0 {
		return "", false
	}
	return reason + "; waiting: " + strings.Join(waiting, ", "), true
}

// checkHerdr probes once and, on a transition, logs it. The banner transition
// is streamed by syncHerdrBanner.
func (s *Server) checkHerdr() {
	changed := s.herdr.probe(s.herdrRunner(), s.cfg.Runner.Name() == runnerHerdr)
	if changed {
		s.log.Warn("herdr state changed", "unavailable", s.herdr.isUnavailable())
	}
	s.syncHerdrBanner()
	if changed && !s.herdr.isUnavailable() {
		s.flushPauseFold()
	}
}

// syncHerdrBanner streams the banner appearing or going away, with a chat
// notice. The need changes with the queue and the runs, so the claim loop calls
// it too.
func (s *Server) syncHerdrBanner() {
	text, show := s.herdrBanner()
	if !s.herdr.setShown(show) {
		return
	}
	if show {
		s.events.publish(EventHerdrUnavailable, "")
		s.chat.Notice(ralphloop.ServerNotice{Kind: EventHerdrUnavailable, Emoji: "🔌", Title: "herdr unavailable", Detail: text})
		return
	}
	s.events.publish(EventHerdrAvailable, "")
	if !s.herdr.isUnavailable() {
		s.chat.Notice(ralphloop.ServerNotice{Kind: EventHerdrAvailable, Emoji: "✅", Title: "herdr back", Detail: "herdr answers again"})
	}
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
