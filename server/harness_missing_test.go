package server_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

// A queued ticket whose resolved harness is not on PATH waits with a reason
// that names the program, what chose it and how to unblock it.
func TestClaimHold_HarnessMissingHoldsTicket(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		agent    string // frontmatter agent:
		queueAs  string
		options  []func(*server.Config)
		missing  string
		wantFrom string
		wantTail string
	}{
		{"queue item", "", "codex", nil, "codex", "the queue item", "requeue with --agent <other>"},
		{"frontmatter", "codex", "claude", nil, "codex", "the ticket's agent", "edit agent: in the ticket"},
		{"global default", "", "", []func(*server.Config){servertest.WithDefaultHarness("codex")}, "codex", "the global default", "requeue with --agent <other>"},
		{"claude", "", "claude", nil, "claude", "the queue item", "requeue with --agent <other>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, repo := t.TempDir(), testutil.TempRepo(t)
			servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "one", servertest.TicketOpts{Agent: tc.agent})
			servertest.SetProjectRepo(t, store, "proj", repo)
			pty := namedFake{runnerfake.NewRunner(), "pty"}
			opts := append([]func(*server.Config){servertest.WithRunner(pty)}, tc.options...)
			h := servertest.StartWithStore(t, store, opts...)
			t.Cleanup(func() { pty.SetState("epic-a-iter-01", agentrunner.StateDone, "") })
			h.Programs.Missing(tc.missing)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", tc.queueAs); err != nil || res.Refused {
				t.Fatalf("add: %+v, %v", res, err)
			}

			var ex server.Explanation
			var err error
			servertest.WaitFor(t, 10*time.Second, "ticket held", func() bool {
				ex, err = h.Client.Explain(ctx, "proj:epic-a/01")
				return err == nil && ex.Verdict == server.VerdictHarnessMissing
			})
			for _, want := range []string{tc.missing + " is not on PATH", tc.wantFrom, tc.wantTail} {
				if !strings.Contains(ex.Reason, want) {
					t.Errorf("reason = %q, want it to contain %q", ex.Reason, want)
				}
			}
			if got := pty.Prompts("epic-a-iter-01"); len(got) != 0 {
				t.Errorf("held ticket launched: %v", got)
			}

			h.Programs.Install(tc.missing)
			servertest.WaitFor(t, 10*time.Second, "ticket launches once installed", func() bool {
				return len(pty.Prompts("epic-a-iter-01")) > 0
			})
		})
	}
}

// An interactive launch whose harness is not on PATH is refused with a message
// naming the program, and makes no session and no worktree.
func TestCreateSession_HarnessMissingRefused(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	if err := servertest.WriteEpic(store, "proj", ""); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, servertest.WithDefaultHarness("codex"))
	h.Programs.Missing("codex")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	res, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{Project: "proj", Worktree: "feature"})
	if err != nil || !res.Refused || res.Reason != server.ReasonHarnessMissing {
		t.Fatalf("create = %+v, %v; want a harness-missing refusal", res, err)
	}
	for _, want := range []string{"codex is not on PATH", "the global default", "Install it or change the default harness"} {
		if !strings.Contains(res.Message, want) {
			t.Errorf("message = %q, want it to contain %q", res.Message, want)
		}
	}
	list, err := h.Client.Sessions(ctx)
	if err != nil || len(list) != 0 {
		t.Errorf("sessions = %v, %v; want none", list, err)
	}
}
