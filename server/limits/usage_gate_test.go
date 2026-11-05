package limits_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

var withUsageSample = servertest.WithUsageSample

func TestUsageGate_HoldsClaudeClaimsOnly(t *testing.T) {
	t.Parallel()
	resets := time.Now().Add(time.Hour).Truncate(time.Second)
	window := func(pct float64) *nativerunner.UsageWindow {
		return &nativerunner.UsageWindow{UsedPercentage: pct, ResetsAt: resets.Unix()}
	}
	cases := []struct {
		name string
		rec  nativerunner.UsageRecord
		want string // "" means not gated
	}{
		{"five-hour at 98", nativerunner.UsageRecord{SeenAt: time.Now(), FiveHour: window(98)}, "Claude usage: 98% of the 5-hour limit, resets " + resets.UTC().Format(time.RFC3339)},
		{"weekly at 98", nativerunner.UsageRecord{SeenAt: time.Now(), FiveHour: window(10), SevenDay: window(98)}, "Claude usage: 98% of the weekly limit, resets " + resets.UTC().Format(time.RFC3339)},
		{"97 does not gate", nativerunner.UsageRecord{SeenAt: time.Now(), FiveHour: window(97), SevenDay: window(97)}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			store, repo := t.TempDir(), testutil.TempRepo(t)
			for _, epic := range []string{"epic-a", "epic-b"} {
				servertest.WriteTicket(t, store, "proj", epic, "01", epic, "")
			}
			servertest.SetProjectRepo(t, store, "proj", repo)
			pty := servertest.NewNamedFake(runnerfake.NewRunner(), "pty")
			h := servertest.StartWithStore(t, store,
				servertest.WithRunnerFor(pty, func(string, string) agentrunner.Runner { return pty }),
				withUsageSample(t, c.rec))
			t.Cleanup(func() {
				for _, label := range []string{"epic-a-iter-01", "epic-b-iter-01"} {
					if len(pty.Prompts(label)) > 0 {
						pty.SetState(label, agentrunner.StateDone, "")
					}
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
				t.Fatalf("add claude: %+v, %v", res, err)
			}
			if res, err := h.Client.Queue.Add(ctx, "proj:epic-b/01", "codex"); err != nil || res.Refused {
				t.Fatalf("add codex: %+v, %v", res, err)
			}
			servertest.WaitFor(t, 10*time.Second, "the codex ticket claims", func() bool { return len(pty.Prompts("epic-b-iter-01")) > 0 })
			if c.want == "" {
				servertest.WaitFor(t, 10*time.Second, "the claude ticket claims", func() bool { return len(pty.Prompts("epic-a-iter-01")) > 0 })
				return
			}
			servertest.WaitFor(t, 10*time.Second, "the claude ticket explains the gate", func() bool {
				ex, err := h.Client.Reads.Explain(ctx, "proj:epic-a/01")
				return err == nil && strings.Contains(ex.Verdict, c.want)
			})
			if n := len(pty.Prompts("epic-a-iter-01")); n != 0 {
				t.Errorf("claude ticket got %d prompts while gated", n)
			}
		})
	}
}

func TestUsageGate_OverrideLetsAClaudeTicketClaim(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "epic-a", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	pty := servertest.NewNamedFake(runnerfake.NewRunner(), "pty")
	rec := nativerunner.UsageRecord{SeenAt: time.Now(), FiveHour: &nativerunner.UsageWindow{UsedPercentage: 99, ResetsAt: time.Now().Add(time.Hour).Unix()}}
	h := servertest.StartWithStore(t, store,
		servertest.WithRunnerFor(pty, func(string, string) agentrunner.Runner { return pty }),
		withUsageSample(t, rec))
	t.Cleanup(func() {
		if len(pty.Prompts("epic-a-iter-01")) > 0 {
			pty.SetState("epic-a-iter-01", agentrunner.StateDone, "")
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	if n := len(pty.Prompts("epic-a-iter-01")); n != 0 {
		t.Fatalf("claimed %d prompts while gated", n)
	}
	res, err := h.Client.Admin.UsageOverride(ctx, "")
	if err != nil || res.Refused {
		t.Fatalf("override: %+v, %v", res, err)
	}
	servertest.WaitFor(t, 10*time.Second, "the claim lands after the override", func() bool { return len(pty.Prompts("epic-a-iter-01")) > 0 })
	res, err = h.Client.Admin.UsageOverride(ctx, "")
	if err != nil || !res.Refused || res.Reason != wire.ReasonNotGated {
		t.Errorf("second override = %+v, %v; want refused %s", res, err, wire.ReasonNotGated)
	}
}

func TestUsageGate_LiftsAtResetWithNoNewSample(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "epic-a", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	pty := servertest.NewNamedFake(runnerfake.NewRunner(), "pty")
	resets := time.Now().Add(3 * time.Second)
	rec := nativerunner.UsageRecord{SeenAt: time.Now(), FiveHour: &nativerunner.UsageWindow{UsedPercentage: 99, ResetsAt: resets.Unix()}}
	h := servertest.StartWithStore(t, store,
		servertest.WithRunnerFor(pty, func(string, string) agentrunner.Runner { return pty }),
		withUsageSample(t, rec))
	t.Cleanup(func() {
		if len(pty.Prompts("epic-a-iter-01")) > 0 {
			pty.SetState("epic-a-iter-01", agentrunner.StateDone, "")
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	if n := len(pty.Prompts("epic-a-iter-01")); n != 0 {
		t.Fatalf("claimed %d prompts while gated", n)
	}
	servertest.WaitFor(t, 15*time.Second, "the claim lands after resets_at", func() bool { return len(pty.Prompts("epic-a-iter-01")) > 0 })
	if time.Now().Before(resets.Truncate(time.Second)) {
		t.Errorf("claimed before resets_at %s", resets)
	}
}
