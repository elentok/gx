package launchtest_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
)

// A session launched on a ticket whose earlier iteration left a summary is
// told that summary, whatever the ticket's type.
func TestLaunch_RelaunchPromptListsEarlierSummaries(t *testing.T) {
	t.Parallel()
	for _, typ := range []string{"implement", "verify", "merge"} {
		t.Run(typ, func(t *testing.T) {
			t.Parallel()
			store, repo := t.TempDir(), testutil.TempRepo(t)
			servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "first", servertest.TicketOpts{Type: typ})
			servertest.SetProjectRepo(t, store, "proj", repo)
			path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			body := schema.AppendIterationSummary(string(raw), schema.IterationSummary{
				SessionID: "sess-old", Time: time.Now(), Label: "agent",
				Did: "wired the parser", WentWrong: "flaky fixture", Decisions: "kept the table", Left: "docs",
			})
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}

			fake := runnerfake.NewRunner()
			h := servertest.StartWithStore(t, store, servertest.WithRunner(rigs.NewNamedFake(fake, "pty")))
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
				t.Fatalf("add: %+v, %v", res, err)
			}
			t.Cleanup(func() { fake.SetState(rigs.ResumeLabel, agentrunner.StateDone, "") })
			servertest.WaitFor(t, 10*time.Second, "the ticket launches", func() bool { return len(fake.Prompts(rigs.ResumeLabel)) > 0 })
			got := fake.Prompts(rigs.ResumeLabel)[0]
			for _, want := range []string{"## Iteration summaries", "sess-old (agent)", "wired the parser", "flaky fixture", "kept the table", "left: docs"} {
				if !strings.Contains(got, want) {
					t.Errorf("prompt lacks %q:\n%s", want, got)
				}
			}
		})
	}
}

// An agent that finishes without a summary still finishes, and gx writes its
// own entry listing what it did.
func TestFinish_NoAgentSummaryStillFinishesAndGxWritesItsEntry(t *testing.T) {
	t.Parallel()
	f := servertest.RetroFixture(t)
	scratch := filepath.Join(f.Store, "proj")
	for _, typ := range []events.Type{events.PromptNudged, events.HumanRelaunch} {
		if err := runlog.AppendEvent(scratch, servertest.SchedEpic, runlog.Event{Type: string(typ), Ticket: "01"}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootCompleted, "")

	raw, err := os.ReadFile(filepath.Join(f.Issues, "01-t.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket not done:\n%s", raw)
	}
	got := schema.ParseIterationSummaries(schema.ParseBody(string(raw)))
	if len(got) != 1 || got[0].Label != "gx" {
		t.Fatalf("summaries = %+v, want one gx entry", got)
	}
	for _, want := range []string{"1 prompt resends", "1 relaunches"} {
		if !strings.Contains(got[0].Did, want) {
			t.Errorf("gx entry did %q lacks %q", got[0].Did, want)
		}
	}
}
