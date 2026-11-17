package launchtest_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
			t.Cleanup(func() { fake.Finish(rigs.ResumeLabel) })
			servertest.WaitState(t, 10*time.Second, "the ticket launches", func() (bool, string) {
				logged, _, err := runlog.ReadEvents(filepath.Join(store, "proj"), "epic-a")
				prompts := fake.Prompts(rigs.ResumeLabel)
				return len(prompts) > 0, fmt.Sprintf("run log (err %v): %+v\nruns: %+v\nprompts %s: %q\nserver log tail:\n%s",
					err, logged, h.Server.Runs(), rigs.ResumeLabel, prompts, h.LogTail(30))
			})
			if got := fake.Prompts(rigs.ResumeLabel)[0]; strings.Contains(got, "## Iteration summaries") || strings.Contains(got, "\n") {
				t.Errorf("typed prompt carries the note:\n%s", got)
			}
			noteFile := filepath.Join(store, "proj", "epic-a", "launch", "01-note.md")
			note, err := os.ReadFile(noteFile)
			if err != nil {
				t.Fatalf("note file: %v", err)
			}
			for _, want := range []string{"## Iteration summaries", "sess-old (agent)", "wired the parser", "flaky fixture", "kept the table", "left: docs"} {
				if !strings.Contains(string(note), want) {
					t.Errorf("note lacks %q:\n%s", want, note)
				}
			}
			args := fake.Args(rigs.ResumeLabel)
			var files []string
			for i, a := range args {
				if a == "--append-system-prompt" {
					t.Errorf("inline prompt flag in %v", args)
				}
				if a == "--append-system-prompt-file" && i+1 < len(args) {
					files = append(files, args[i+1])
				}
			}
			if len(files) != 1 {
				t.Fatalf("prompt files = %v, want exactly one", files)
			}
			prompt, err := os.ReadFile(files[0])
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(prompt), noteFile) || strings.Contains(string(prompt), "\n") {
				t.Errorf("prompt file should be one line naming %s:\n%s", noteFile, prompt)
			}
		})
	}
}

// A run where gx never stepped in says so.
func TestFinish_GxEntryWentWrongNoneRecorded(t *testing.T) {
	t.Parallel()
	f := servertest.RetroFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootCompleted, "")

	raw, err := os.ReadFile(filepath.Join(f.Issues, "01-t.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := schema.ParseIterationSummaries(schema.ParseBody(string(raw)))
	if len(got) != 1 || got[0].Label != "gx" {
		t.Fatalf("summaries = %+v, want one gx entry", got)
	}
	if got[0].WentWrong != "none recorded" {
		t.Errorf("gx entry went wrong = %q, want none recorded", got[0].WentWrong)
	}
}

// An agent that finishes without a summary still finishes, and gx writes its
// own entry listing what it did.
func TestFinish_NoAgentSummaryStillFinishesAndGxWritesItsEntry(t *testing.T) {
	t.Parallel()
	f := servertest.RetroFixture(t)
	scratch := filepath.Join(f.Store, "proj")
	for _, typ := range []events.Type{
		events.PromptNudged, events.PromptNudged, events.HumanRelaunch, events.LaunchFailed,
		events.RecoveryEscalated, events.FinishBlocked, events.SkillsRefreshed, events.LaunchRetried,
	} {
		ev := runlog.Event{Type: string(typ), Ticket: "01"}
		if events.KindRequired(typ) {
			ev.Kind = string(events.AgentNameTaken)
		}
		if err := runlog.AppendEvent(scratch, servertest.SchedEpic, ev); err != nil {
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
	for _, want := range []string{"2 prompt resends", "1 relaunches"} {
		if !strings.Contains(got[0].Did, want) {
			t.Errorf("gx entry did %q lacks %q", got[0].Did, want)
		}
	}
	const wantWrong = "2 prompt resends, 1 relaunch, 1 launch failure, 1 recovery escalation, 1 finish block, 1 skill refresh, 1 skill retry"
	if got[0].WentWrong != wantWrong {
		t.Errorf("gx entry went wrong = %q, want %q", got[0].WentWrong, wantWrong)
	}
}
