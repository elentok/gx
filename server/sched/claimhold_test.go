package sched_test

import (
	"context"
	"errors"
	"github.com/elentok/gx/server/servertest/rigs"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

// Under agent-runner: pty, a Codex ticket launches on pty and herdr being down
// is no business of anyone's: no hold, no banner.
func TestHerdrBanner_HiddenForCodexTicketOnPty(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "one", servertest.TicketOpts{Agent: "codex"})
	servertest.SetProjectRepo(t, store, "proj", repo)
	pty := rigs.NewNamedFake(runnerfake.NewRunner(), "pty")
	herdr := rigs.NewNamedFake(runnerfake.NewRunner(), "herdr")
	herdr.SetHealthErr(errors.New("socket closed"))
	h := servertest.StartWithStore(t, store, servertest.WithRunnerFor(pty, func(name, _ string) agentrunner.Runner {
		if name == "herdr" {
			return herdr
		}
		return pty
	}), func(c *server.Config) { c.HerdrRetryInterval = 20 * time.Millisecond })
	t.Cleanup(func() { pty.SetState("epic-a-iter-01", agentrunner.StateDone, "") })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "codex"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	servertest.WaitFor(t, 10*time.Second, "codex ticket launches on pty", func() bool {
		return len(pty.Prompts("epic-a-iter-01")) > 0
	})
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snap.HerdrUnavailable || snap.HerdrText != "" {
		t.Errorf("banner = %v %q, want none", snap.HerdrUnavailable, snap.HerdrText)
	}
	if got := herdr.Prompts("epic-a-iter-01"); len(got) != 0 {
		t.Errorf("codex ticket reached herdr: %v", got)
	}
}

// With PTY configured and herdr down, a queue holding no herdr ticket shows no
// banner and streams no herdr event.
func TestHerdrBanner_HiddenWhileNoTicketNeedsHerdr(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-b", "01", "claude one", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	pty := rigs.NewNamedFake(runnerfake.NewRunner(), "pty")
	herdr := rigs.NewNamedFake(runnerfake.NewRunner(), "herdr")
	herdr.SetHealthErr(errors.New("socket closed"))
	h := servertest.StartWithStore(t, store, servertest.WithRunnerFor(pty, func(name, _ string) agentrunner.Runner {
		if name == "herdr" {
			return herdr
		}
		return pty
	}), func(c *server.Config) { c.HerdrRetryInterval = 20 * time.Millisecond })
	t.Cleanup(func() { pty.SetState("epic-b-iter-01", agentrunner.StateDone, "") })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-b/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add claude: %+v, %v", res, err)
	}
	servertest.WaitFor(t, 10*time.Second, "claude ticket launches on pty", func() bool {
		return len(pty.Prompts("epic-b-iter-01")) > 0
	})
	time.Sleep(100 * time.Millisecond) // negative wait: a herdr event would show within a few retries
	for done := false; !done; {
		select {
		case ev := <-evs:
			if ev.Type == wire.EventHerdrUnavailable || ev.Type == wire.EventHerdrAvailable {
				t.Errorf("unexpected herdr event %+v", ev)
			}
		default:
			done = true
		}
	}
	if snap, err := h.Client.Snapshot(ctx); err != nil || snap.HerdrUnavailable || snap.HerdrText != "" {
		t.Errorf("snapshot banner = %v %q (err %v), want none", snap.HerdrUnavailable, snap.HerdrText, err)
	}
	if got, err := h.Client.Handshake(ctx); err != nil || got.HerdrUnavailable {
		t.Errorf("handshake = %+v (err %v), want no banner", got, err)
	}
}
