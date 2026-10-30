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

// A herdr outage holds the tickets that launch on herdr (Codex) and nobody else.
func TestClaimHold_HerdrOutageHoldsOnlyHerdrTickets(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		err        error
		wantReason string
		wantEvent  bool
	}{
		{"down", errors.New("socket closed"), "herdr is down", true},
		{"not on PATH", fmt.Errorf("$ herdr workspace list: %w", exec.ErrNotFound), "not found", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, repo := t.TempDir(), testutil.TempRepo(t)
			servertest.WriteTicket(t, store, "proj", "epic-a", "01", "codex one", "")
			servertest.WriteTicket(t, store, "proj", "epic-b", "01", "claude one", "")
			servertest.SetProjectRepo(t, store, "proj", repo)
			pty := namedFake{runnerfake.NewRunner(), "pty"}
			herdr := namedFake{runnerfake.NewRunner(), "herdr"}
			herdr.SetHealthErr(tc.err)
			h := servertest.StartWithStore(t, store, servertest.WithRunnerFor(pty, func(name, _ string) agentrunner.Runner {
				switch name {
				case "pty":
					return pty
				case "herdr":
					return herdr
				}
				return nil
			}), func(c *server.Config) { c.HerdrRetryInterval = 20 * time.Millisecond })
			t.Cleanup(func() {
				herdr.SetState("epic-a-iter-01", agentrunner.StateDone, "")
				pty.SetState("epic-b-iter-01", agentrunner.StateDone, "")
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

			servertest.WaitFor(t, 10*time.Second, "claude ticket launches on pty", func() bool {
				return len(pty.Prompts("epic-b-iter-01")) > 0
			})
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
			snap, err := h.Client.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if snap.HerdrUnavailable != tc.wantEvent {
				t.Errorf("snapshot HerdrUnavailable = %v, want %v", snap.HerdrUnavailable, tc.wantEvent)
			}

			herdr.SetHealthErr(nil)
			servertest.WaitFor(t, 10*time.Second, "codex ticket launches once herdr answers", func() bool {
				return len(herdr.Prompts("epic-a-iter-01")) > 0
			})
			sawDown := false
			for drained := false; !drained; {
				select {
				case ev := <-evs:
					sawDown = sawDown || ev.Type == server.EventHerdrUnavailable
				default:
					drained = true
				}
			}
			if sawDown != tc.wantEvent {
				t.Errorf("herdr-unavailable event = %v, want %v", sawDown, tc.wantEvent)
			}
		})
	}
}
