package rigs

import (
	"context"
	"github.com/elentok/gx/server/servertest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

// stopCounter counts Stop calls on the headless runner.
type stopCounter struct {
	*servertest.TurnRunner
	stops atomic.Int32
}

func (r *stopCounter) Stop(s agentrunner.Session) error {
	r.stops.Add(1)
	return r.TurnRunner.Stop(s)
}

// ZeroCommitPaused is 01 paused zero-commit, its agent still alive.
type ZeroCommitPaused struct {
	H          *servertest.Harness
	Store      string
	WtPath     string
	TicketPath string
	Pause      runlog.Event
	// AgentGone makes the runner's Find miss the agent.
	AgentGone func()
	// Stops counts runner Stops from now on.
	Stops func() int32
}

func PauseZeroCommit(t *testing.T, headless bool, opts ...func(*server.Config)) ZeroCommitPaused {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	p := ZeroCommitPaused{Store: store, TicketPath: filepath.Join(store, "proj/epic-a/issues/01-first.md")}
	if headless {
		fake := &stopCounter{TurnRunner: servertest.NewTurnRunner(func(string, string) {})}
		p.H = servertest.StartWithStore(t, store, append([]func(*server.Config){servertest.WithRunner(fake)}, opts...)...)
		if res, err := p.H.Client.Queue.Add(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
			t.Fatalf("add: %+v, %v", res, err)
		}
		servertest.WaitStatus(t, p.TicketPath, schema.StatusLegacyAnswer)
		p.AgentGone = func() {
			if sess, ok, _ := fake.Find("epic-a-iter-01"); ok {
				_ = fake.Runner.Stop(sess)
			}
		}
		base := fake.stops.Load()
		p.Stops = func() int32 { return fake.stops.Load() - base }
	} else {
		p.H = servertest.StartWithStore(t, store, opts...)
		p.H.OnTurn(func(turn *servertest.Turn) { turn.Transcript("done") })
		if res, err := p.H.Client.Queue.Add(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
			t.Fatalf("add: %+v, %v", res, err)
		}
		servertest.WaitStatus(t, p.TicketPath, schema.StatusLegacyAnswer)
		p.AgentGone = func() { p.H.KillAgents(t, servertest.EpicA01) }
		base := p.H.Stops()
		p.Stops = func() int32 { return int32(p.H.Stops() - base) }
	}
	// The pause writes the ticket's status before its run-log event.
	servertest.Await(t, func() bool {
		var ok bool
		p.Pause, ok = runlog.LatestPauseEvent(filepath.Join(store, "proj"), "epic-a", "01")
		return ok
	})
	r, err := git.FindRepo(repo)
	if err != nil {
		t.Fatal(err)
	}
	wts, err := git.ListWorktrees(*r)
	if err != nil {
		t.Fatal(err)
	}
	if p.WtPath = history.WorktreePathForBranch("ralph-loop/epic-a-item-01", wts); p.WtPath == "" {
		t.Fatalf("no iteration worktree in %+v", wts)
	}
	return p
}

// Report writes the agent's late report onto the ticket and pings a rescan.
func (p ZeroCommitPaused) Report(t *testing.T, mutate func(*schema.Ticket)) {
	t.Helper()
	if err := schema.UpdateTicket(p.TicketPath, mutate); err != nil {
		t.Fatal(err)
	}
	if err := p.H.Client.Tickets.Changed(context.Background(), "proj:epic-a/01"); err != nil {
		t.Fatal(err)
	}
}

func (p ZeroCommitPaused) FinishedLate(commitless bool) func(*schema.Ticket) {
	return func(tk *schema.Ticket) {
		tk.IterationStatus = schema.IterationStatusFinished
		tk.IterationFinishedAt = p.Pause.Time.Add(time.Second)
		tk.Commitless = commitless
	}
}

// Adopted waits for 01 to be done and returns its late-finish-adopted reason
// as the history shows it.
func (p ZeroCommitPaused) Adopted(t *testing.T) string {
	t.Helper()
	servertest.WaitStatus(t, p.TicketPath, schema.StatusDone)
	var reason string
	servertest.Await(t, func() bool {
		hist, err := p.H.Client.Reads.History(context.Background(), "proj:epic-a/01")
		if err != nil {
			return false
		}
		for _, e := range hist.Events {
			if e.Type == string(events.LateFinishAdopted) {
				reason = e.Reason
				return true
			}
		}
		return false
	})
	return reason
}

// StaysPaused gives a scan time to act, then checks it did not.
func (p ZeroCommitPaused) StaysPaused(t *testing.T) {
	t.Helper()
	time.Sleep(200 * time.Millisecond) // negative wait: a few 50ms polls
	tk, err := schema.ParseTicket(p.TicketPath)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status == schema.StatusDone {
		t.Errorf("status = done, want left alone")
	}
	hist, err := p.H.Client.Reads.History(context.Background(), "proj:epic-a/01")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range hist.Events {
		if e.Type == string(events.LateFinishAdopted) {
			t.Errorf("history has %s: %s", e.Type, e.Reason)
		}
	}
}

func PausedAt(p ZeroCommitPaused) string { return p.Pause.Time.UTC().Format(time.RFC3339) }
