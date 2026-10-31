package server_test

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

// Under agent-runner: herdr, a herdr outage holds every ticket, whatever its agent.
func TestClaimHold_HerdrOutageHoldsHerdrTickets(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		err        error
		wantReason string
	}{
		{"down", errors.New("socket closed"), "herdr is down"},
		{"not on PATH", fmt.Errorf("$ herdr workspace list: %w", exec.ErrNotFound), "not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, repo := t.TempDir(), testutil.TempRepo(t)
			servertest.WriteTicket(t, store, "proj", "epic-a", "01", "codex one", "")
			servertest.WriteTicket(t, store, "proj", "epic-b", "01", "claude one", "")
			servertest.SetProjectRepo(t, store, "proj", repo)
			herdr := namedFake{runnerfake.NewRunner(), "herdr"}
			herdr.SetHealthErr(tc.err)
			h := servertest.StartWithStore(t, store, servertest.WithRunnerFor(herdr, func(name, _ string) agentrunner.Runner {
				if name == "herdr" {
					return herdr
				}
				return nil
			}), func(c *server.Config) { c.HerdrRetryInterval = 20 * time.Millisecond })
			t.Cleanup(func() {
				herdr.SetState("epic-a-iter-01", agentrunner.StateDone, "")
				herdr.SetState("epic-b-iter-01", agentrunner.StateDone, "")
			})
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			evs, err := h.Client.Events(ctx, 0)
			if err != nil {
				t.Fatal(err)
			}
			if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "codex"); err != nil || res.Refused {
				t.Fatalf("add codex: %+v, %v", res, err)
			}
			if res, err := h.Client.QueueAdd(ctx, "proj:epic-b/01", "claude"); err != nil || res.Refused {
				t.Fatalf("add claude: %+v, %v", res, err)
			}

			var ex server.Explanation
			servertest.WaitFor(t, 10*time.Second, "codex ticket held", func() bool {
				ex, err = h.Client.Explain(ctx, "proj:epic-a/01")
				return err == nil && ex.Verdict == server.VerdictHerdrDown
			})
			if !strings.Contains(ex.Reason, tc.wantReason) {
				t.Errorf("reason = %q, want it to contain %q", ex.Reason, tc.wantReason)
			}
			if got := herdr.Prompts("epic-a-iter-01"); len(got) != 0 {
				t.Errorf("held codex ticket launched: %v", got)
			}
			if got := herdr.Prompts("epic-b-iter-01"); len(got) != 0 {
				t.Errorf("held claude ticket launched: %v", got)
			}
			snap, err := h.Client.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !snap.HerdrUnavailable || !strings.Contains(snap.HerdrText, tc.wantReason) || !strings.Contains(snap.HerdrText, "proj:epic-a/01") || !strings.Contains(snap.HerdrText, "proj:epic-b/01") {
				t.Errorf("snapshot banner = %v %q, want it on, naming both tickets and %q", snap.HerdrUnavailable, snap.HerdrText, tc.wantReason)
			}

			// The event follows the next banner sync, so recover only after it
			// arrives; recovering first can hide the banner before it streams.
			for sawDown := false; !sawDown; {
				select {
				case ev := <-evs:
					sawDown = ev.Type == server.EventHerdrUnavailable
				case <-ctx.Done():
					t.Fatal("no herdr-unavailable event for the waiting codex ticket")
				}
			}
			herdr.SetHealthErr(nil)
			servertest.WaitFor(t, 10*time.Second, "tickets launch once herdr answers", func() bool {
				return len(herdr.Prompts("epic-a-iter-01")) > 0 && len(herdr.Prompts("epic-b-iter-01")) > 0
			})
		})
	}
}

// Under agent-runner: pty, a Codex ticket launches on pty and herdr being down
// is no business of anyone's: no hold, no banner.
func TestHerdrBanner_HiddenForCodexTicketOnPty(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "one", servertest.TicketOpts{Agent: "codex"})
	servertest.SetProjectRepo(t, store, "proj", repo)
	pty := namedFake{runnerfake.NewRunner(), "pty"}
	herdr := namedFake{runnerfake.NewRunner(), "herdr"}
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
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "codex"); err != nil || res.Refused {
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
	pty := namedFake{runnerfake.NewRunner(), "pty"}
	herdr := namedFake{runnerfake.NewRunner(), "herdr"}
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
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-b/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add claude: %+v, %v", res, err)
	}
	servertest.WaitFor(t, 10*time.Second, "claude ticket launches on pty", func() bool {
		return len(pty.Prompts("epic-b-iter-01")) > 0
	})
	time.Sleep(100 * time.Millisecond) // negative wait: a herdr event would show within a few retries
	for done := false; !done; {
		select {
		case ev := <-evs:
			if ev.Type == server.EventHerdrUnavailable || ev.Type == server.EventHerdrAvailable {
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
