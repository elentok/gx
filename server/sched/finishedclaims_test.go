package sched_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// A restart settles a claimed ticket whose agent already reported finished,
// even when its pane still reads working (a leftover background task).
func TestReclaim_FinishedReportSettlesAWorkingPane(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake := servertest.NewHeadlessFake(nativerunner.VerdictLive)
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake))
	wtPath, ticketPath := leaveIteration(t, h, store, repo, "fake")
	if _, err := fake.Start(agentrunner.StartOptions{Label: "epic-a-iter-01", Cwd: wtPath}); err != nil {
		t.Fatal(err)
	}
	fake.SetState("epic-a-iter-01", agentrunner.StateWorking, "")
	asHeadlessRun(t, h)
	if err := schema.UpdateTicket(ticketPath, func(tk *schema.Ticket) {
		tk.IterationStatus = schema.IterationStatusFinished
		tk.IterationFinishedAt = time.Now()
	}); err != nil {
		t.Fatal(err)
	}

	h.Restart(t)
	waitStatus(t, ticketPath, schema.StatusDone)
}

func TestPauseStuckFinishedClaims(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	h := servertest.StartWithStore(t, store)
	epics, err := tickets.Load(store + "/proj")
	if err != nil {
		t.Fatal(err)
	}
	path := epics[0].Tickets[0].Path
	finishedAt := time.Now().Add(-time.Hour)
	if err := schema.UpdateTicket(path, func(tk *schema.Ticket) {
		tk.Status = schema.StatusClaimed
		tk.IterationStatus = schema.IterationStatusFinished
		tk.IterationFinishedAt = finishedAt
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.Client.Tickets.Changed(context.Background(), "proj:epic-a/01"); err != nil {
		t.Fatal(err)
	}

	h.Server.Scheduler().PauseStuckFinishedClaims(finishedAt.Add(time.Minute))
	if tk, err := schema.ParseTicket(path); err != nil || tk.Status != schema.StatusClaimed {
		t.Fatalf("within the grace: status = %q, err = %v, want claimed", tk.Status, err)
	}

	h.Server.Scheduler().PauseStuckFinishedClaims(finishedAt.Add(time.Hour))
	tk, err := schema.ParseTicket(path)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != schema.StatusPaused {
		t.Errorf("past the grace: status = %q, want paused", tk.Status)
	}

	reclaim := func() {
		t.Helper()
		if err := schema.UpdateTicket(path, func(tk *schema.Ticket) {
			tk.Status = schema.StatusClaimed
			tk.IterationStatus = schema.IterationStatusFinished
			tk.IterationFinishedAt = finishedAt
		}); err != nil {
			t.Fatal(err)
		}
		if err := h.Client.Tickets.Changed(context.Background(), "proj:epic-a/01"); err != nil {
			t.Fatal(err)
		}
	}
	past := finishedAt.Add(time.Hour)

	reclaim()
	h.Server.Registry().PutRun("proj:epic-a", runs.Run{Address: "proj:epic-a/01", Runner: "pty"})
	h.Server.Registry().MarkSettling("proj:epic-a/01")
	h.Server.Scheduler().PauseStuckFinishedClaims(past)
	if tk, err := schema.ParseTicket(path); err != nil || tk.Status != schema.StatusClaimed {
		t.Fatalf("settling run: status = %q, err = %v, want claimed", tk.Status, err)
	}

	h.Server.Registry().PutRun("proj:epic-a", runs.Run{Address: "proj:epic-a/01", Runner: "pty"})
	h.Server.Scheduler().PauseStuckFinishedClaims(past)
	if tk, err := schema.ParseTicket(path); err != nil || tk.Status != schema.StatusClaimed {
		t.Fatalf("tracked run in a wait: status = %q, err = %v, want claimed", tk.Status, err)
	}

	h.Server.Registry().Delete("proj:epic-a/01")
	h.Server.Scheduler().PauseStuckFinishedClaims(past)
	tk, err = schema.ParseTicket(path)
	if err != nil || tk.Status != schema.StatusPaused {
		t.Fatalf("untracked run: status = %q, err = %v, want paused", tk.Status, err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "agent reported finished at " + finishedAt.UTC().Format(time.RFC3339) + " but the server no longer tracks the run"; !strings.Contains(string(body), want) {
		t.Errorf("ticket lacks %q:\n%s", want, body)
	}
}
