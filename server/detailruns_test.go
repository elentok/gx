package server_test

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
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
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "work", "---\nstatus: open\n---\n", map[string]string{
			"01-a.md": issueMD("01", status, "", "A"),
		})
		withRepo(t, store)
		for _, ev := range evs {
			if err := ralphloop.AppendEvent(filepath.Join(store, "proj"), "work", ev); err != nil {
				t.Fatal(err)
			}
		}
	})
	return fetchDetail(t, h, base, "proj:work/01").Runs
}

func TestTicketDetail_RunsWithNormalEnds(t *testing.T) {
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
	start := runEv(events.IterationStarted, 0, nil)
	for status, want := range map[string]string{"claimed": server.RunRunning, "open": server.RunAgentLost} {
		got := runsOf(t, status, start)
		if len(got) != 1 || got[0].Outcome != want || got[0].Duration != 0 {
			t.Fatalf("status %s: runs = %+v, want one %s run with no duration", status, got, want)
		}
	}
}

func TestTicketDetail_NeverClaimedHasNoRuns(t *testing.T) {
	if got := runsOf(t, "open"); len(got) != 0 {
		t.Fatalf("runs = %+v", got)
	}
}
