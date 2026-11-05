package sched

import (
	"io"
	"log/slog"
	"testing"

	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop/notify"
)

// fakeRunner is a runner that only has a name.
type fakeRunner struct {
	agentrunner.Runner
	name string
}

func (r fakeRunner) Name() string { return r.name }

type fakeHost struct {
	runner     agentrunner.Runner
	configured bool
	waiting    []string
	events     []string
	notices    []string
	flushed    int
}

func (h *fakeHost) Runner() agentrunner.Runner { return h.runner }
func (h *fakeHost) Configured() bool           { return h.configured }
func (h *fakeHost) Waiting() []string          { return h.waiting }
func (h *fakeHost) Publish(event, _ string)    { h.events = append(h.events, event) }
func (h *fakeHost) Notice(n notify.ServerNotice) {
	h.notices = append(h.notices, n.Detail)
}
func (h *fakeHost) FlushPauseFold() { h.flushed++ }

func newWatch(h *fakeHost) *HerdrWatch {
	return NewHerdrWatch(slog.New(slog.NewTextHandler(io.Discard, nil)), h, 0)
}

func TestHerdrWatch_BannerShowsOnlyWhileDownAndNeeded(t *testing.T) {
	t.Parallel()
	h := &fakeHost{configured: true}
	w := newWatch(h)
	w.SetUnavailable(true)

	if _, show := w.Banner(); show {
		t.Fatal("banner shown with no ticket needing herdr")
	}
	h.waiting = []string{"proj:epic/01"}
	text, show := w.Banner()
	if !show || text != "herdr is down; waiting: proj:epic/01" {
		t.Fatalf("banner = %q, %v", text, show)
	}
	h.configured = false
	if _, show := w.Banner(); show {
		t.Fatal("banner shown although herdr is not the configured runner")
	}
}

func TestHerdrWatch_SyncBannerStreamsEachTransitionOnce(t *testing.T) {
	t.Parallel()
	h := &fakeHost{configured: true, waiting: []string{"proj:epic/01"}}
	w := newWatch(h)
	w.SetUnavailable(true)

	w.SyncBanner()
	w.SyncBanner()
	w.SetUnavailable(false)
	w.SyncBanner()

	want := []string{wire.EventHerdrUnavailable, wire.EventHerdrAvailable}
	if len(h.events) != 2 || h.events[0] != want[0] || h.events[1] != want[1] {
		t.Fatalf("events = %v, want %v", h.events, want)
	}
	if len(h.notices) != 2 {
		t.Fatalf("notices = %v, want one per transition", h.notices)
	}
}

func TestHerdrWatch_HoldsOnlyHerdrRunners(t *testing.T) {
	t.Parallel()
	w := newWatch(&fakeHost{})
	w.SetUnavailable(true)

	if reason, held := w.Holds(fakeRunner{name: runs.RunnerHerdr}); !held || reason != "herdr is down" {
		t.Errorf("herdr runner = %q, %v, want held", reason, held)
	}
	if _, held := w.Holds(fakeRunner{name: "pty"}); held {
		t.Error("pty runner held by herdr")
	}
	if _, held := w.Holds(nil); held {
		t.Error("nil runner held by herdr")
	}
}
