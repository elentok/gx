package server_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

func startHarnessClaim(t *testing.T, store string, extra ...func(*server.Config)) (*servertest.Harness, *runnerfake.Runner, string) {
	t.Helper()
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	fake := runnerfake.NewRunner()
	path := filepath.Join(t.TempDir(), "config.json")
	opts := append([]func(*server.Config){servertest.WithRunner(fake), servertest.WithConfigFile(path), func(c *server.Config) {
		c.MaxAgentsPerRoot = 1
	}}, extra...)
	h := servertest.StartWithStore(t, store, opts...)
	servertest.WriteConfig(t, path, store, nil)
	return h, fake, path
}

func switchDefaultHarness(t *testing.T, h *servertest.Harness, path, store, harness string) {
	t.Helper()
	servertest.WriteConfig(t, path, store, map[string]any{"agents": map[string]any{"default-harness": harness}})
	if res, err := h.Client.ReloadConfig(context.Background()); err != nil || res.Refused {
		t.Fatalf("reload: %+v, %v", res, err)
	}
}

func waitKind(t *testing.T, fake *runnerfake.Runner, label string) agentrunner.Kind {
	t.Helper()
	var kind agentrunner.Kind
	servertest.WaitFor(t, 10*time.Second, label+" starts", func() bool {
		kind = fake.Kind(label)
		return kind != ""
	})
	return kind
}

// A waiting follows-default ticket claims on the default read at claim time.
func TestHarnessClaim_WaitingTicketFollowsDefaultAtClaimTime(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	h, fake, path := startHarnessClaim(t, store)
	ctx := context.Background()
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", ""); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	if k := waitKind(t, fake, "epic-a-iter-01"); k != "claude" {
		t.Fatalf("first ticket started as %q, want claude", k)
	}

	switchDefaultHarness(t, h, path, store, "codex")
	fake.SetState("epic-a-iter-01", agentrunner.StateDone, "")
	if k := waitKind(t, fake, "epic-a-iter-02"); k != "codex" {
		t.Errorf("waiting ticket started as %q, want codex", k)
	}
	if k := fake.Kind("epic-a-iter-01"); k != "claude" {
		t.Errorf("running session changed to %q, want claude", k)
	}
}

// A retry keeps the harness of the earlier run; a run with no transcript does
// not count; the ticket's own agent: beats the earlier run.
func TestHarnessClaim_RelaunchKeepsEarlierRunHarness(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		ticketAgent string
		transcript  bool
		want        agentrunner.Kind
	}{
		{"earlier claude run keeps claude", "", true, "claude"},
		{"run without a transcript follows the default", "", false, "codex"},
		{"frontmatter beats the earlier run", "codex", true, "codex"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := t.TempDir()
			servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "one", servertest.TicketOpts{Agent: tc.ticketAgent})
			h, fake, path := startHarnessClaim(t, store)
			servertest.SeedEarlierClaudeRun(t, h, store, "proj", "epic-a", "01", "sess-1", tc.transcript)
			switchDefaultHarness(t, h, path, store, "codex")
			if res, err := h.Client.TicketRelaunch(context.Background(), "proj:epic-a/01", false); err != nil || res.Refused {
				t.Fatalf("relaunch: %+v, %v", res, err)
			}
			if k := waitKind(t, fake, "epic-a-iter-01"); k != tc.want {
				t.Errorf("relaunch started as %q, want %q", k, tc.want)
			}
		})
	}
}
