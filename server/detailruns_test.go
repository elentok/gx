package server_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
)

var runsT0 = time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)

// runEv is an event of proj:work/01 at runsT0 plus min minutes.
func runEv(typ events.Type, min int, mod func(*ralphloop.Event)) ralphloop.Event {
	ev := ralphloop.Event{Type: string(typ), Ticket: "01", Address: "proj:work/01", Time: runsT0.Add(time.Duration(min) * time.Minute)}
	if mod != nil {
		mod(&ev)
	}
	return ev
}

func runsOf(t *testing.T, status string, evs ...ralphloop.Event) []server.DetailRun {
	t.Helper()
	return runsWith(t, status, "", nil, nil, evs...)
}

// runsWith is runsOf for a ticket with extra frontmatter, pause answers in its
// log at the given minutes, and a hook on the ticket file once it is written.
func runsWith(t *testing.T, status, extra string, answerMins []int, prep func(ticketPath string), evs ...ralphloop.Event) []server.DetailRun {
	t.Helper()
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "work", "---\nstatus: open\n---\n", map[string]string{
			"01-a.md": issueMD("01", status, extra, "A"),
		})
		withRepo(t, store)
		for _, ev := range evs {
			if err := ralphloop.AppendEvent(filepath.Join(store, "proj"), "work", ev); err != nil {
				t.Fatal(err)
			}
		}
		path := filepath.Join(store, "proj", "work", "issues", "01-a.md")
		for _, min := range answerMins {
			ev := tickets.LogEvent{Type: tickets.EventPauseAnswer, TS: runsT0.Add(time.Duration(min) * time.Minute), Choice: "go"}
			if _, err := tickets.AppendLog(path, ev); err != nil {
				t.Fatal(err)
			}
		}
		if prep != nil {
			prep(path)
		}
	})
	return fetchDetail(t, h, base, "proj:work/01").Runs
}

func runsAt(min int) time.Time { return runsT0.Add(time.Duration(min) * time.Minute) }

func TestTicketDetail_RunsWithNoNormalEnd(t *testing.T) {
	t.Parallel()
	cold := func(ev *ralphloop.Event) { ev.Path = "cold-start" }
	got := runsOf(t, "open",
		runEv(events.IterationStarted, 0, cold),
		runEv(events.ClaimReopened, 2, nil),
		runEv(events.IterationStarted, 10, cold),
		runEv(events.IterationStarted, 15, cold),
		runEv(events.HumanRelaunch, 18, nil),
		runEv(events.IterationStarted, 20, cold),
		runEv(events.Reclaimed, 22, nil),
		runEv(events.CherryPicked, 30, nil),
	)
	want := []server.DetailRun{
		{Start: runsAt(0), Duration: 120, Outcome: server.RunAgentGoneAfterRestart, StartKind: server.StartNewSession},
		{Start: runsAt(10), Duration: 300, Outcome: server.RunAgentLost, StartKind: server.StartNewSession},
		{Start: runsAt(15), Duration: 180, Outcome: server.RunRelaunched, StartKind: server.StartNewSession},
		{Start: runsAt(20), Duration: 600, Outcome: server.RunMerged, StartKind: server.StartNewSession},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("runs = %+v\nwant %+v", got, want)
	}
}

func TestTicketDetail_CancelledRunEndsAtStatusWrite(t *testing.T) {
	t.Parallel()
	prep := func(path string) {
		if err := os.Chtimes(path, runsAt(50), runsAt(50)); err != nil {
			t.Fatal(err)
		}
	}
	got := runsWith(t, "cancelled", "", nil, prep, runEv(events.IterationStarted, 40, nil))
	want := []server.DetailRun{{Start: runsAt(40), Duration: 600, Outcome: server.RunCancelled, StartKind: server.StartNewSession}}
	if !slices.Equal(got, want) {
		t.Fatalf("runs = %+v\nwant %+v", got, want)
	}
}

func TestTicketDetail_AnswerToLiveAgentStartsSameSessionRun(t *testing.T) {
	t.Parallel()
	paused := func(ev *ralphloop.Event) { ev.Kind = "merge-conflict" }
	got := runsWith(t, "open", "", []int{5},
		nil,
		runEv(events.IterationStarted, 0, nil),
		runEv(events.Paused, 3, paused),
		runEv(events.CherryPicked, 12, nil),
	)
	want := []server.DetailRun{
		{Start: runsAt(0), Duration: 180, Outcome: server.RunPaused, PauseLabel: "Merge conflict", StartKind: server.StartNewSession},
		{Start: runsAt(5), Duration: 420, Outcome: server.RunMerged, StartKind: server.StartSameSession},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("runs = %+v\nwant %+v", got, want)
	}
}

func TestTicketDetail_AnswerThenFreshStartAddsNoExtraRun(t *testing.T) {
	t.Parallel()
	paused := func(ev *ralphloop.Event) { ev.Kind = "merge-conflict" }
	got := runsWith(t, "open", "", []int{5}, nil,
		runEv(events.IterationStarted, 0, nil),
		runEv(events.Paused, 3, paused),
		runEv(events.IterationStarted, 8, nil),
		runEv(events.CherryPicked, 12, nil),
	)
	if len(got) != 2 || got[1].Start != runsAt(8) || got[1].Outcome != server.RunMerged {
		t.Fatalf("runs = %+v", got)
	}
}

func TestTicketDetail_AnswerToLiveAgentStillRunning(t *testing.T) {
	t.Parallel()
	paused := func(ev *ralphloop.Event) { ev.Kind = "merge-conflict" }
	extra := "claimed_at: " + runsAt(5).Format(time.RFC3339) + "\n"
	got := runsWith(t, "claimed", extra, []int{5}, nil,
		runEv(events.IterationStarted, 0, nil),
		runEv(events.Paused, 3, paused),
	)
	if len(got) != 2 || got[1].Start != runsAt(5) || got[1].Outcome != server.RunRunning || got[1].StartKind != server.StartSameSession {
		t.Fatalf("runs = %+v", got)
	}
}

func TestTicketDetail_LateFinishAdoptionStartsRunUntilMerge(t *testing.T) {
	t.Parallel()
	paused := func(ev *ralphloop.Event) { ev.Kind = "merge-conflict" }
	got := runsOf(t, "done",
		runEv(events.IterationStarted, 0, nil),
		runEv(events.Paused, 3, paused),
		runEv(events.LateFinishAdopted, 20, nil),
		runEv(events.CherryPicked, 21, nil),
	)
	want := []server.DetailRun{
		{Start: runsAt(0), Duration: 180, Outcome: server.RunPaused, PauseLabel: "Merge conflict", StartKind: server.StartNewSession},
		{Start: runsAt(20), Duration: 60, Outcome: server.RunMerged, StartKind: server.StartSameSession},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("runs = %+v\nwant %+v", got, want)
	}
}

func TestTicketDetail_RunsWithNormalEnds(t *testing.T) {
	t.Parallel()
	resumed := func(ev *ralphloop.Event) { ev.Path = "resumed" }
	paused := func(ev *ralphloop.Event) { ev.Kind = "merge-conflict" }
	got := runsOf(t, "open",
		runEv(events.IterationStarted, 0, func(ev *ralphloop.Event) { ev.Path = "cold-start" }),
		runEv(events.ConflictHit, 1, nil),
		runEv(events.Nudged, 2, nil),
		runEv(events.PausedSmartZone, 3, nil),
		runEv(events.RecoveryApplied, 4, func(ev *ralphloop.Event) { ev.Kind = string(events.SelfReported) }),
		runEv(events.CherryPicked, 10, nil),
		runEv(events.IterationStarted, 20, resumed),
		runEv(events.Commitless, 25, nil),
		runEv(events.IterationStarted, 30, resumed),
		runEv(events.Paused, 33, paused),
		runEv(events.IterationStarted, 40, nil),
		runEv(events.TicketStartedOver, 41, nil),
	)
	at := func(min int) time.Time { return runsT0.Add(time.Duration(min) * time.Minute) }
	want := []server.DetailRun{
		{Start: at(0), Duration: 600, Outcome: server.RunMerged, StartKind: server.StartNewSession},
		{Start: at(20), Duration: 300, Outcome: server.RunMergedNoCommits, StartKind: server.StartSameSession},
		{Start: at(30), Duration: 180, Outcome: server.RunPaused, PauseLabel: "Merge conflict", StartKind: server.StartSameSession},
		{Start: at(40), Duration: 60, Outcome: server.RunStartedOver, StartKind: server.StartNewSession},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("runs = %+v\nwant %+v", got, want)
	}
}

func TestTicketDetail_OpenLastRun(t *testing.T) {
	t.Parallel()
	start := runEv(events.IterationStarted, 0, nil)
	for status, want := range map[string]string{"claimed": server.RunRunning, "open": server.RunAgentLost} {
		got := runsOf(t, status, start)
		if len(got) != 1 || got[0].Outcome != want || got[0].Duration != 0 {
			t.Fatalf("status %s: runs = %+v, want one %s run with no duration", status, got, want)
		}
	}
}

func TestTicketDetail_NeverClaimedHasNoRuns(t *testing.T) {
	t.Parallel()
	if got := runsOf(t, "open"); len(got) != 0 {
		t.Fatalf("runs = %+v", got)
	}
}
