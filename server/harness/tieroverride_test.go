package harness_test

import (
	"context"
	"github.com/elentok/gx/server/servertest/rigs"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
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
	rigs.WriteProjectAgents(t, store, repo, tierProjectAgents)
	rec := &startRecorder{Runner: runnerfake.NewRunner()}
	h := servertest.StartWithStore(t, store, servertest.WithRunner(rec))
	ctx := context.Background()

	res, err := h.Client.Admin.Repair(ctx, "resume", wire.RepairRequest{Address: "proj:epic-a/01", Tier: "strongest"})
	if err != nil || res.Refused {
		t.Fatalf("resume = %+v, %v", res, err)
	}
	if tk, err := schema.ParseTicket(path); err != nil || tk.TierOverride != "strongest" {
		t.Fatalf("after resume: override = %q, %v; want strongest", tk.TierOverride, err)
	}
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	servertest.WaitFor(t, 20*time.Second, "the first launch", func() bool { return rec.count() >= 1 })
	if !rigs.HasFlag(rec.args(0), "--model", "opus") {
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
	rigs.WriteProjectAgents(t, store, repo, tierProjectAgents)
	rec := &startRecorder{Runner: runnerfake.NewRunner()}
	h := servertest.StartWithStore(t, store, servertest.WithRunner(rec))
	ctx := context.Background()

	if res, err := h.Client.Admin.Repair(ctx, "resume", wire.RepairRequest{Address: "proj:epic-a/01"}); err != nil || res.Refused {
		t.Fatalf("resume = %+v, %v", res, err)
	}
	if tk, err := schema.ParseTicket(path); err != nil || tk.TierOverride != "" {
		t.Fatalf("override = %q, %v; want none", tk.TierOverride, err)
	}
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	servertest.WaitFor(t, 20*time.Second, "the launch", func() bool { return rec.count() >= 1 })
	if !rigs.HasFlag(rec.args(0), "--model", "sonnet") {
		t.Errorf("launch args = %q, want --model sonnet", rec.args(0))
	}
}

func TestLaunch_EmptyTierModelPausesModelMismatchAndStartsNothing(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "first", servertest.TicketOpts{Type: string(schema.TypeImplement)})
	path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	rigs.WriteProjectAgents(t, store, repo, `,"agents":{"claude":{"effort":"low"}}`)
	rec := &startRecorder{Runner: runnerfake.NewRunner()}
	// A model-mismatch pause reaches chat, not the OS banner, so a fake Slack
	// webhook stands in for the banner.
	var hookMu sync.Mutex
	var hookBodies []string
	hook := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		hookMu.Lock()
		defer hookMu.Unlock()
		if !strings.Contains(string(b), "server started") {
			hookBodies = append(hookBodies, string(b))
		}
	}))
	t.Cleanup(hook.Close)
	hookCount := func() int {
		hookMu.Lock()
		defer hookMu.Unlock()
		return len(hookBodies)
	}
	h := servertest.StartWithStore(t, store, servertest.WithRunner(rec), func(c *server.Config) {
		c.AgentsClaude = config.AgentConfig{}
		c.RecoverySettings.FollowUps = "proj:follow-ups"
		c.Settings.Chat.SlackWebhookURL = hook.URL
		c.Settings.Chat.GateStatePath = filepath.Join(t.TempDir(), "gate.json")
		c.Settings.Chat.FlushInterval = 20 * time.Millisecond
	})

	if res, err := h.Client.Queue.Add(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	servertest.WaitFor(t, 20*time.Second, "the model-mismatch pause", func() bool {
		tk, err := schema.ParseTicket(path)
		return err == nil && tk.Status == schema.StatusPaused && tk.PauseReason == schema.PauseModelMismatch
	})
	tk, _ := schema.ParseTicket(path)
	if tk.ModelRan != "no model" || tk.ModelExpected != "a model from its tier" {
		t.Errorf("model fields = %q / %q, want no model / a model from its tier", tk.ModelRan, tk.ModelExpected)
	}
	var drafts []string
	servertest.WaitFor(t, 20*time.Second, "the pause message and the follow-up draft", func() bool {
		drafts, _ = filepath.Glob(filepath.Join(store, "proj", "follow-ups", "issues", "*model-mismatch*.md"))
		return len(drafts) > 0 && hookCount() > 0
	})
	if n := hookCount(); n != 1 {
		hookMu.Lock()
		t.Errorf("chat pause messages = %d, want 1: %q", n, hookBodies)
		hookMu.Unlock()
	}
	if len(drafts) != 1 {
		t.Fatalf("follow-up drafts = %v, want 1", drafts)
	}
	draft, _ := os.ReadFile(drafts[0])
	if !strings.Contains(string(draft), "its tier resolved to no model") {
		t.Errorf("follow-up has no empty-model source:\n%s", draft)
	}
	if n := rec.count(); n != 0 {
		t.Errorf("starts = %d, want 0", n)
	}
}

func TestResume_InvalidTierIsRefusedAndChangesNothing(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	path := pausedOnModel(t, store)
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)

	res, err := h.Client.Admin.Repair(context.Background(), "resume", wire.RepairRequest{Address: "proj:epic-a/01", Tier: "huge"})
	if err != nil || !res.Refused || res.Reason != wire.ReasonBadTier {
		t.Fatalf("resume = %+v, %v; want refusal %s", res, err, wire.ReasonBadTier)
	}
	if tk, err := schema.ParseTicket(path); err != nil || tk.Status != schema.StatusPaused || tk.TierOverride != "" {
		t.Errorf("ticket = %+v, %v; want still paused with no override", tk, err)
	}
}
