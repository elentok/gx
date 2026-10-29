package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

const runEpic = "epic-a"

// launchOnRunners queues one ticket and waits until a runner of the pair holds its
// session, returning the runner that did.
func launchOnRunners(t *testing.T, h *servertest.Harness, runners ...*runnerfake.Runner) *runnerfake.Runner {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	var got *runnerfake.Runner
	servertest.WaitFor(t, 30*time.Second, "a launch on one of the runners", func() bool {
		for _, r := range runners {
			if ss, _ := r.List(runEpic); len(ss) > 0 {
				got = r
				return true
			}
		}
		return false
	})
	return got
}

func startRunnerOptions(t *testing.T, opts ...func(*server.Config)) *servertest.Harness {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	return servertest.StartWithStore(t, store, opts...)
}

func TestServertest_WithRunnerLaunchesOnTheFake(t *testing.T) {
	t.Parallel()
	fake := runnerfake.NewRunner()
	h := startRunnerOptions(t, servertest.WithRunner(fake))
	if got := launchOnRunners(t, h, fake); got != fake {
		t.Fatalf("launched on %v, want the fake", got)
	}
}

func TestServertest_WithRunnerForRoutesTheRunToFn(t *testing.T) {
	t.Parallel()
	configured, routed := runnerfake.NewRunner(), runnerfake.NewRunner()
	h := startRunnerOptions(t, servertest.WithRunnerFor(configured, func(string, string) agentrunner.Runner { return routed }))
	if got := launchOnRunners(t, h, configured, routed); got != routed {
		t.Fatalf("launched on the configured runner, want the routed one")
	}
	if ss, _ := configured.List(runEpic); len(ss) != 0 {
		t.Errorf("configured runner holds %d sessions, want 0", len(ss))
	}
}

// Whatever the order of the options, a runner choice beats WithHerdr and the
// harness still has its fake herdr.
func TestServertest_RunnerChoiceBeatsWithHerdrInEveryOrder(t *testing.T) {
	t.Parallel()
	herdr := servertest.WithHerdr(servertest.HerdrIsTheSubject)
	cases := map[string]func(fake *runnerfake.Runner) []func(*server.Config){
		"runner first": func(f *runnerfake.Runner) []func(*server.Config) {
			return []func(*server.Config){servertest.WithRunner(f), herdr}
		},
		"herdr first": func(f *runnerfake.Runner) []func(*server.Config) {
			return []func(*server.Config){herdr, servertest.WithRunner(f)}
		},
		"runner-for first": func(f *runnerfake.Runner) []func(*server.Config) {
			return []func(*server.Config){servertest.WithRunnerFor(f, func(string, string) agentrunner.Runner { return f }), herdr}
		},
		"herdr then runner-for": func(f *runnerfake.Runner) []func(*server.Config) {
			return []func(*server.Config){herdr, servertest.WithRunnerFor(f, func(string, string) agentrunner.Runner { return f })}
		},
	}
	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fake := runnerfake.NewRunner()
			h := startRunnerOptions(t, mk(fake)...)
			if h.Herdr == nil {
				t.Fatal("h.Herdr is nil")
			}
			if got := launchOnRunners(t, h, fake); got != fake {
				t.Fatalf("launched on %v, want the fake", got)
			}
		})
	}
}
