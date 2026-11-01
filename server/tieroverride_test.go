package server_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
)

// startRecorder records the args of every session the server starts, in order.
type startRecorder struct {
	*runnerfake.Runner
	mu     sync.Mutex
	starts [][]string
}

func (r *startRecorder) Start(opts agentrunner.StartOptions) (agentrunner.Session, error) {
	r.mu.Lock()
	r.starts = append(r.starts, append([]string(nil), opts.Args...))
	r.mu.Unlock()
	return r.Runner.Start(opts)
}

func (r *startRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.starts)
}

func (r *startRecorder) args(i int) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.starts[i]
}

const tierProjectAgents = `,"agents":{"claude":{"model":"sonnet","effort":"low","strongest":{"model":"opus","effort":"high"}}}`

func pausedOnModel(t *testing.T, store string) string {
	t.Helper()
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "first", servertest.TicketOpts{PauseReason: string(schema.PauseModelMismatch)})
	return filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
}

func TestResume_TierOverrideLaunchesOnceOnThatTier(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	path := pausedOnModel(t, store)
	writeProjectAgents(t, store, repo, tierProjectAgents)
	rec := &startRecorder{Runner: runnerfake.NewRunner()}
	h := servertest.StartWithStore(t, store, servertest.WithRunner(rec))
	ctx := context.Background()

	res, err := h.Client.Repair(ctx, "resume", server.RepairRequest{Address: "proj:epic-a/01", Tier: "strongest"})
	if err != nil || res.Refused {
		t.Fatalf("resume = %+v, %v", res, err)
	}
	if tk, err := schema.ParseTicket(path); err != nil || tk.TierOverride != "strongest" {
		t.Fatalf("after resume: override = %q, %v; want strongest", tk.TierOverride, err)
	}
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	servertest.WaitFor(t, 20*time.Second, "the first launch", func() bool { return rec.count() >= 1 })
	if !hasFlag(rec.args(0), "--model", "opus") {
		t.Errorf("first launch args = %q, want --model opus", rec.args(0))
	}
	if tk, err := schema.ParseTicket(path); err != nil || tk.TierOverride != "" {
		t.Errorf("after launch: override = %q, %v; want cleared", tk.TierOverride, err)
	}
}

func TestResume_WithoutTierLeavesTheWorkerModel(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	path := pausedOnModel(t, store)
	writeProjectAgents(t, store, repo, tierProjectAgents)
	rec := &startRecorder{Runner: runnerfake.NewRunner()}
	h := servertest.StartWithStore(t, store, servertest.WithRunner(rec))
	ctx := context.Background()

	if res, err := h.Client.Repair(ctx, "resume", server.RepairRequest{Address: "proj:epic-a/01"}); err != nil || res.Refused {
		t.Fatalf("resume = %+v, %v", res, err)
	}
	if tk, err := schema.ParseTicket(path); err != nil || tk.TierOverride != "" {
		t.Fatalf("override = %q, %v; want none", tk.TierOverride, err)
	}
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	servertest.WaitFor(t, 20*time.Second, "the launch", func() bool { return rec.count() >= 1 })
	if !hasFlag(rec.args(0), "--model", "sonnet") {
		t.Errorf("launch args = %q, want --model sonnet", rec.args(0))
	}
}

func TestResume_InvalidTierIsRefusedAndChangesNothing(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	path := pausedOnModel(t, store)
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)

	res, err := h.Client.Repair(context.Background(), "resume", server.RepairRequest{Address: "proj:epic-a/01", Tier: "huge"})
	if err != nil || !res.Refused || res.Reason != server.ReasonBadTier {
		t.Fatalf("resume = %+v, %v; want refusal %s", res, err, server.ReasonBadTier)
	}
	if tk, err := schema.ParseTicket(path); err != nil || tk.Status != schema.StatusPaused || tk.TierOverride != "" {
		t.Errorf("ticket = %+v, %v; want still paused with no override", tk, err)
	}
}
