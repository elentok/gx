package servertest

import (
	"slices"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil/runnerfake"
)

// HeadlessFake is a fake runner with the headless runner's reattach, cleanup
// and prune hooks.
type HeadlessFake struct {
	*runnerfake.Runner
	verdict nativerunner.Verdict

	mu        sync.Mutex
	cleaned   []string
	stopped   []string
	retention time.Duration
	paused    map[string]bool
}

// NewHeadlessFake is a HeadlessFake whose Reattach answers verdict.
func NewHeadlessFake(verdict nativerunner.Verdict) *HeadlessFake {
	return &HeadlessFake{Runner: runnerfake.NewRunner(), verdict: verdict}
}

// Cleaned lists the labels Cleanup was called with.
func (h *HeadlessFake) Cleaned() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.cleaned)
}

// Stopped lists the IDs of the sessions Stop was called with.
func (h *HeadlessFake) Stopped() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.stopped)
}

// PruneSeen is the retention and the paused labels the last Prune saw; paused
// is nil until Prune has run.
func (h *HeadlessFake) PruneSeen() (paused map[string]bool, retention time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.paused, h.retention
}

func (h *HeadlessFake) Stop(s agentrunner.Session) error {
	h.mu.Lock()
	h.stopped = append(h.stopped, s.ID)
	h.mu.Unlock()
	return h.Runner.Stop(s)
}

func (h *HeadlessFake) Reattach(label string) (agentrunner.Session, nativerunner.Verdict, error) {
	if s, ok, _ := h.Find(label); ok {
		return s, h.verdict, nil
	}
	return agentrunner.Session{Label: label, ID: "p1"}, h.verdict, nil
}

func (h *HeadlessFake) Cleanup(label string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleaned = append(h.cleaned, label)
	return nil
}

func (h *HeadlessFake) Prune(maxAge time.Duration, _ time.Time, paused func(string) bool) ([]string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.retention = maxAge
	// Called once per project; only the proj call sees these labels.
	if h.paused == nil {
		h.paused = map[string]bool{}
	}
	for _, l := range []string{"epic-a-iter-01", "epic-a-iter-02"} {
		h.paused[l] = h.paused[l] || paused(l)
	}
	return nil, nil
}

// RenamedFake is a HeadlessFake answering to name, so two of them are two runners.
type RenamedFake struct {
	*HeadlessFake
	name string
}

func (r RenamedFake) Name() string { return r.name }

// NewRenamedFake is a RenamedFake named name whose Reattach answers verdict.
func NewRenamedFake(name string, verdict nativerunner.Verdict) RenamedFake {
	return RenamedFake{NewHeadlessFake(verdict), name}
}
