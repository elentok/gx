package server_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

// runMergeTicket runs the merge ticket to its first settle and returns it.
func runMergeTicket(t *testing.T, turn func(f *schedFixture, cwd string) string) (*schedFixture, schema.Ticket) {
	t.Helper()
	f, _ := newMergeFixtureWith(t, func(f *schedFixture, cwd string, _ int) string { return turn(f, cwd) })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	for {
		select {
		case <-ctx.Done():
			t.Fatal("merge ticket never settled")
		case ev := <-evs:
			if ev.Type != server.EventTicketDone && ev.Type != server.EventIterationParked {
				continue
			}
			tk, err := schema.ParseTicket(f.ticketFile(t, "01"))
			if err != nil {
				t.Fatal(err)
			}
			f.assertFeatureKept(t, "settle")
			return f, tk
		}
	}
}

// advanceTrunk commits name on the repo's own checkout, the trunk.
func advanceTrunk(t *testing.T, f *schedFixture, name, content string) {
	t.Helper()
	testutil.WriteFile(t, f.repo, name, content)
	testutil.CommitAll(t, f.repo, "trunk "+name)
}

func assertParked(t *testing.T, f *schedFixture, tk schema.Ticket, status schema.Status, kind events.Kind, reason string) {
	t.Helper()
	if tk.Status != status || string(tk.ParkKind) != string(kind) {
		t.Fatalf("ticket = %s/%s, want %s/%s:\n%s", tk.Status, tk.ParkKind, status, kind, readFile(t, f.ticketFile(t, "01")))
	}
	if body := readFile(t, f.ticketFile(t, "01")); !strings.Contains(body, reason) {
		t.Errorf("ticket lacks reason %q:\n%s", reason, body)
	}
}

func TestMergeFinish_FixerCommitOnTheEpicStillFinishesDone(t *testing.T) {
	_, tk := runMergeTicket(t, func(_ *schedFixture, cwd string) string {
		testutil.WriteFile(t, cwd, "fix.txt", "fix")
		testutil.CommitAll(t, cwd, "epic: Fix typecheck/lint after rebase onto main")
		return "finished"
	})
	if tk.Status != schema.StatusDone {
		t.Errorf("status = %s, want done", tk.Status)
	}
}

func TestMergeFinish_TargetNotAnAncestorParksNeedsRepair(t *testing.T) {
	f, tk := runMergeTicket(t, func(f *schedFixture, _ string) string {
		advanceTrunk(t, f, "trunk.txt", "moved on")
		return "finished"
	})
	assertParked(t, f, tk, schema.StatusNeedsRepair, events.IterationError, "is not an ancestor of "+schedEpic)
}

func TestMergeFinish_RebaseInProgressParksNeedsRepair(t *testing.T) {
	f, tk := runMergeTicket(t, func(f *schedFixture, cwd string) string {
		testutil.WriteFile(t, cwd, "x.txt", "epic")
		testutil.CommitAll(t, cwd, "epic x")
		advanceTrunk(t, f, "x.txt", "trunk")
		trunk := gitOut(t, f.repo, "branch", "--show-current")
		if out, err := exec.Command("git", "-C", cwd, "rebase", trunk).CombinedOutput(); err == nil {
			t.Fatalf("rebase did not stop on a conflict:\n%s", out)
		}
		return "finished"
	})
	assertParked(t, f, tk, schema.StatusNeedsRepair, events.IterationError, "rebase still in progress")
}

func TestMergeFinish_SelfReportedNeedsAnswerParksNeedsAnswer(t *testing.T) {
	f, tk := runMergeTicket(t, func(*schedFixture, string) string { return "needs-answer" })
	assertParked(t, f, tk, schema.StatusNeedsAnswer, events.SelfReported, "")
}

func TestMergeFinish_AgentStoppingWithoutAReportParksNeedsRepair(t *testing.T) {
	f, tk := runMergeTicket(t, func(*schedFixture, string) string { return "" })
	assertParked(t, f, tk, schema.StatusNeedsRepair, events.MergeUnreported, "without reporting")
}

// reportMergeFinishedLate is the parked merge agent reporting finished after
// its park, and the store rescan that follows.
func reportMergeFinishedLate(t *testing.T, f *schedFixture) {
	t.Helper()
	var park ralphloop.Event
	waitFor(t, func() bool {
		var ok bool
		park, ok = ralphloop.LatestParkEvent(filepath.Join(f.store, "proj"), schedEpic, "01")
		return ok
	})
	if err := schema.UpdateTicket(f.ticketFile(t, "01"), func(tk *schema.Ticket) {
		tk.IterationStatus = schema.IterationStatusFinished
		tk.IterationFinishedAt = park.Time.Add(time.Second)
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.h.Client.TicketChanged(context.Background(), "proj:"+schedEpic+"/01"); err != nil {
		t.Fatal(err)
	}
}

func lateFinishReason(t *testing.T, f *schedFixture) (string, bool) {
	t.Helper()
	hist, err := f.h.Client.History(context.Background(), "proj:"+schedEpic+"/01")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range hist.Events {
		if e.Type == string(events.LateFinishAdopted) {
			return e.Reason, true
		}
	}
	return "", false
}

func TestLateFinish_MergeUnreportedClosesDoneOnceChecksPass(t *testing.T) {
	f, _ := runMergeTicket(t, func(*schedFixture, string) string { return "" })
	reportMergeFinishedLate(t, f)
	waitStatus(t, f.ticketFile(t, "01"), schema.StatusDone)
	var reason string
	waitFor(t, func() bool {
		var ok bool
		reason, ok = lateFinishReason(t, f)
		return ok
	})
	if !strings.Contains(reason, "closed merge") {
		t.Errorf("reason = %q, want closed merge", reason)
	}
	f.assertFeatureKept(t, "late finish")
}

func TestLateFinish_MergeUnreportedStaysParkedWhileChecksFail(t *testing.T) {
	f, _ := runMergeTicket(t, func(f *schedFixture, _ string) string {
		advanceTrunk(t, f, "trunk.txt", "moved on")
		return ""
	})
	reportMergeFinishedLate(t, f)
	time.Sleep(300 * time.Millisecond)
	tk, err := schema.ParseTicket(f.ticketFile(t, "01"))
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != schema.StatusNeedsRepair || tk.ParkKind != schema.ParkKindMergeUnreported {
		t.Errorf("ticket = %s/%s, want needs-repair/merge-unreported", tk.Status, tk.ParkKind)
	}
	if reason, ok := lateFinishReason(t, f); ok {
		t.Errorf("history has late-finish-adopted: %s", reason)
	}
}

func TestMergeFinish_BudgetKillParksBudgetKilled(t *testing.T) {
	testBudgetKillParksBudgetKilled(t, func() float64 { return 6 }, servertest.TicketOpts{Type: "merge"})
}
