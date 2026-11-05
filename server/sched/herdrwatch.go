// Package sched is the top of the server's lifecycle: the parts that decide
// when work starts.
package sched

import (
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
)

const defaultHerdrRetryInterval = 30 * time.Second

// HerdrHost is what the herdr watch needs from the server.
type HerdrHost interface {
	// Runner is the runner that hosts herdr tickets, or nil when the server
	// cannot build one.
	Runner() agentrunner.Runner
	// Configured is whether herdr is the configured runner.
	Configured() bool
	// Waiting is the tickets that need herdr: queued ones that launch on it
	// and tracked runs it hosts.
	Waiting() []string
	Publish(event, id string)
	Notice(ralphloop.ServerNotice)
	// FlushPauseFold sends the digest of pauses held back while herdr was down.
	FlushPauseFold()
}

// HerdrWatch tracks whether herdr answers. The server starts either way; this
// only reports the state and the transitions.
type HerdrWatch struct {
	log   *slog.Logger
	host  HerdrHost
	retry time.Duration

	mu          sync.Mutex
	unavailable bool
	// missing is herdr not on PATH. It holds herdr tickets, but only reports
	// herdr unavailable when herdr is the configured runner: a person running
	// PTY without herdr installed has nothing to be told.
	missing bool
	// shown is the banner state last streamed: herdr down while a ticket needs it.
	shown bool
}

// NewHerdrWatch returns a watch that retries every retry (a default when not
// positive).
func NewHerdrWatch(log *slog.Logger, host HerdrHost, retry time.Duration) *HerdrWatch {
	if retry <= 0 {
		retry = defaultHerdrRetryInterval
	}
	return &HerdrWatch{log: log, host: host, retry: retry}
}

// setShown records the banner state and reports whether it changed.
func (w *HerdrWatch) setShown(v bool) (changed bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	changed = v != w.shown
	w.shown = v
	return changed
}

// IsUnavailable reports whether herdr is down.
func (w *HerdrWatch) IsUnavailable() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.unavailable
}

// SetUnavailable forces the down state, for tests.
func (w *HerdrWatch) SetUnavailable(v bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.unavailable = v
}

// holdReason says why herdr tickets wait, or reports that they need not.
func (w *HerdrWatch) holdReason() (string, bool) {
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

// Holds reports why r cannot take work now: r is herdr and herdr is down or
// not installed. Any other runner is never held by herdr.
func (w *HerdrWatch) Holds(r agentrunner.Runner) (string, bool) {
	if r == nil || r.Name() != runs.RunnerHerdr {
		return "", false
	}
	return w.holdReason()
}

// probe records r's host health and reports whether unavailable changed. A
// runner with no host never reports herdr down. configured is whether herdr is
// the configured runner.
func (w *HerdrWatch) probe(r agentrunner.Runner, configured bool) (changed bool) {
	err := agentrunner.Healthy(r)
	missing := errors.Is(err, exec.ErrNotFound)
	down := err != nil && (!missing || configured)
	w.mu.Lock()
	defer w.mu.Unlock()
	changed = down != w.unavailable
	w.unavailable, w.missing = down, missing
	return changed
}

// Banner is the text of the "herdr down" banner, and whether it shows: herdr
// is down or missing and a ticket needs it. Only a herdr config shows it: a
// session herdr left behind reclaims without a banner.
func (w *HerdrWatch) Banner() (string, bool) {
	if !w.host.Configured() {
		return "", false
	}
	reason, down := w.holdReason()
	if !down {
		return "", false
	}
	waiting := w.host.Waiting()
	if len(waiting) == 0 {
		return "", false
	}
	return reason + "; waiting: " + strings.Join(waiting, ", "), true
}

// Check probes once and, on a transition, logs it. The banner transition is
// streamed by SyncBanner.
func (w *HerdrWatch) Check() {
	changed := w.probe(w.host.Runner(), w.host.Configured())
	if changed {
		w.log.Warn("herdr state changed", "unavailable", w.IsUnavailable())
	}
	w.SyncBanner()
	if changed && !w.IsUnavailable() {
		w.host.FlushPauseFold()
	}
}

// SyncBanner streams the banner appearing or going away, with a chat notice.
// The need changes with the queue and the runs, so the claim loop calls it too.
func (w *HerdrWatch) SyncBanner() {
	text, show := w.Banner()
	if !w.setShown(show) {
		return
	}
	if show {
		w.host.Publish(wire.EventHerdrUnavailable, "")
		w.host.Notice(ralphloop.ServerNotice{Kind: wire.EventHerdrUnavailable, Emoji: "🔌", Title: "herdr unavailable", Detail: text})
		return
	}
	w.host.Publish(wire.EventHerdrAvailable, "")
	if !w.IsUnavailable() {
		w.host.Notice(ralphloop.ServerNotice{Kind: wire.EventHerdrAvailable, Emoji: "✅", Title: "herdr back", Detail: "herdr answers again"})
	}
}

// KeepChecked retries herdr until ctx ends, so a herdr that starts late (or
// dies later) is noticed without a server restart.
func (w *HerdrWatch) KeepChecked(ctx context.Context) {
	if _, ok := w.host.Runner().(agentrunner.HealthChecker); !ok {
		return
	}
	ticker := time.NewTicker(w.retry)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.Check()
		}
	}
}
