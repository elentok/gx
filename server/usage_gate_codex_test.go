package server_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

// withCodexUsageSample seeds the persisted Codex usage sample.
func withCodexUsageSample(t *testing.T, rec nativerunner.UsageRecord) func(*server.Config) {
	t.Helper()
	return func(c *server.Config) {
		data, err := json.Marshal(rec)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(c.StateDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(c.StateDir, "usage-sample-codex.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUsageOverride_NoHarnessLiftsTheDefaultHarness(t *testing.T) {
	t.Parallel()
	rec := nativerunner.UsageRecord{SeenAt: time.Now(), FiveHour: &nativerunner.UsageWindow{UsedPercentage: 99, ResetsAt: time.Now().Add(time.Hour).Unix(), Name: "5-hour"}}
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithDefaultHarness("codex"), withCodexUsageSample(t, rec))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if res, err := h.Client.UsageOverride(ctx, ""); err != nil || res.Refused {
		t.Fatalf("override with the default harness codex = %+v, %v; want it lifted", res, err)
	}
}

func TestUsageGate_CodexSampleHoldsCodexClaimsOnlyAndItsOverrideLiftsThem(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	for _, epic := range []string{"epic-a", "epic-b"} {
		servertest.WriteTicket(t, store, "proj", epic, "01", epic, "")
	}
	servertest.SetProjectRepo(t, store, "proj", repo)
	pty := namedFake{runnerfake.NewRunner(), "pty"}
	resets := time.Now().Add(time.Hour).Truncate(time.Second)
	rec := nativerunner.UsageRecord{SeenAt: time.Now(), SevenDay: &nativerunner.UsageWindow{UsedPercentage: 98, ResetsAt: resets.Unix(), Name: "weekly"}}
	h := servertest.StartWithStore(t, store,
		servertest.WithRunnerFor(pty, func(string, string) agentrunner.Runner { return pty }),
		withCodexUsageSample(t, rec))
	t.Cleanup(func() {
		for _, label := range []string{"epic-a-iter-01", "epic-b-iter-01"} {
			if len(pty.Prompts(label)) > 0 {
				pty.SetState(label, agentrunner.StateDone, "")
			}
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "codex"); err != nil || res.Refused {
		t.Fatalf("add codex: %+v, %v", res, err)
	}
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-b/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add claude: %+v, %v", res, err)
	}
	servertest.WaitFor(t, 10*time.Second, "the claude ticket claims", func() bool { return len(pty.Prompts("epic-b-iter-01")) > 0 })
	servertest.WaitFor(t, 10*time.Second, "the codex ticket explains the gate", func() bool {
		ex, err := h.Client.Explain(ctx, "proj:epic-a/01")
		return err == nil && strings.Contains(ex.Verdict, "Codex usage: 98% of the weekly limit, resets "+resets.UTC().Format(time.RFC3339))
	})
	if n := len(pty.Prompts("epic-a-iter-01")); n != 0 {
		t.Fatalf("codex ticket got %d prompts while gated", n)
	}

	if res, err := h.Client.UsageOverride(ctx, "claude"); err != nil || !res.Refused || res.Reason != server.ReasonNotGated {
		t.Errorf("claude override = %+v, %v; want refused %s", res, err, server.ReasonNotGated)
	}
	if res, err := h.Client.UsageOverride(ctx, "no-such"); err == nil {
		t.Errorf("unknown harness override = %+v, want an error", res)
	}
	if res, err := h.Client.UsageOverride(ctx, "codex"); err != nil || res.Refused {
		t.Fatalf("codex override = %+v, %v", res, err)
	}
	servertest.WaitFor(t, 10*time.Second, "the codex claim lands after its override", func() bool { return len(pty.Prompts("epic-a-iter-01")) > 0 })
}
