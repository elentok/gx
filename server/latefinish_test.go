package server_test

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/tickets/schema"
)

// stopCounter counts Stop calls on the headless runner.
type stopCounter struct {
	*turnRunner
	stops atomic.Int32
}

func (r *stopCounter) Stop(s agentrunner.Session) error {
	r.stops.Add(1)
	return r.turnRunner.Stop(s)
}

// zeroCommitParked is 01 parked zero-commit, its agent still alive.
type zeroCommitParked struct {
	h          *servertest.Harness
	store      string
	wtPath     string
	ticketPath string
	park       ralphloop.Event
	// agentGone makes the runner's Find miss the agent.
	agentGone func()
	// stops counts runner Stops from now on.
	stops func() int32
}

func parkZeroCommit(t *testing.T, headless bool) zeroCommitParked {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	p := zeroCommitParked{store: store, ticketPath: filepath.Join(store, "proj/epic-a/issues/01-first.md")}
	if headless {
		fake := &stopCounter{turnRunner: newTurnRunner(func(string, string) {})}
		p.h = servertest.StartWithStore(t, store, func(c *server.Config) {
			c.RunnerFor = func(_, _ string) agentrunner.Runner { return fake }
		})
		if res, err := p.h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
			t.Fatalf("add: %+v, %v", res, err)
		}
		waitStatus(t, p.ticketPath, schema.StatusNeedsAnswer)
		p.agentGone = func() {
			if sess, ok, _ := fake.Find("epic-a-iter-01"); ok {
				_ = fake.Runner.Stop(sess)
			}
		}
		base := fake.stops.Load()
		p.stops = func() int32 { return fake.stops.Load() - base }
	} else {
		p.h = servertest.StartWithStore(t, store)
		runToZeroCommitPark(t, p.h, "done")
		p.agentGone = func() {
			p.h.Herdr.Register("agent", "get", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
				return nil, herdrfake.Identities{}, servertest.AgentNotFound(argv[2])
			})
		}
		var closes atomic.Int32
		p.h.Herdr.Register("tab", "close", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
			closes.Add(1)
			return map[string]any{}, herdrfake.Identities{}, nil
		})
		p.stops = func() int32 { return closes.Load() }
	}
	// The park writes the ticket's status before its run-log event.
	waitFor(t, func() bool {
		var ok bool
		p.park, ok = ralphloop.LatestParkEvent(filepath.Join(store, "proj"), "epic-a", "01")
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
	if p.wtPath = git.WorktreePathForBranch("ralph-loop/epic-a-item-01", wts); p.wtPath == "" {
		t.Fatalf("no iteration worktree in %+v", wts)
	}
	return p
}

// report writes the agent's late report onto the ticket and pings a rescan.
func (p zeroCommitParked) report(t *testing.T, mutate func(*schema.Ticket)) {
	t.Helper()
	if err := schema.UpdateTicket(p.ticketPath, mutate); err != nil {
		t.Fatal(err)
	}
	if err := p.h.Client.TicketChanged(context.Background(), "proj:epic-a/01"); err != nil {
		t.Fatal(err)
	}
}

func (p zeroCommitParked) finishedLate(commitless bool) func(*schema.Ticket) {
	return func(tk *schema.Ticket) {
		tk.IterationStatus = schema.IterationStatusFinished
		tk.FinishedAt = p.park.Time.Add(time.Second)
		tk.Commitless = commitless
	}
}

// adopted waits for 01 to be done and returns its late-finish-adopted reason
// as the history shows it.
func (p zeroCommitParked) adopted(t *testing.T) string {
	t.Helper()
	waitStatus(t, p.ticketPath, schema.StatusDone)
	var reason string
	waitFor(t, func() bool {
		hist, err := p.h.Client.History(context.Background(), "proj:epic-a/01")
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

// staysParked gives a scan time to act, then checks it did not.
func (p zeroCommitParked) staysParked(t *testing.T) {
	t.Helper()
	time.Sleep(300 * time.Millisecond)
	tk, err := schema.ParseTicket(p.ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status == schema.StatusDone {
		t.Errorf("status = done, want left alone")
	}
	hist, err := p.h.Client.History(context.Background(), "proj:epic-a/01")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range hist.Events {
		if e.Type == string(events.LateFinishAdopted) {
			t.Errorf("history has %s: %s", e.Type, e.Reason)
		}
	}
}

func parkedAt(p zeroCommitParked) string { return p.park.Time.UTC().Format(time.RFC3339) }

func TestLateFinish_LandsLateCommit(t *testing.T) {
	runnerShapes(t, func(t *testing.T, headless bool) {
		p := parkZeroCommit(t, headless)
		commitAgentWork(t, p.wtPath)
		p.report(t, p.finishedLate(false))
		reason := p.adopted(t)
		sha := ralphloop.LatestLandedSHA(filepath.Join(p.store, "proj"), "epic-a", "01")
		if sha == "" || !strings.Contains(reason, "landed "+sha) || !strings.Contains(reason, "parked "+parkedAt(p)) {
			t.Errorf("reason = %q, want parked %s and landed %q", reason, parkedAt(p), sha)
		}
		if p.stops() == 0 {
			t.Error("live agent not stopped after landing")
		}
	})
}

func TestLateFinish_ClosesCommitless(t *testing.T) {
	runnerShapes(t, func(t *testing.T, headless bool) {
		p := parkZeroCommit(t, headless)
		p.report(t, p.finishedLate(true))
		if reason := p.adopted(t); !strings.Contains(reason, "closed commitless") {
			t.Errorf("reason = %q, want closed commitless", reason)
		}
	})
}

// With the agent gone the adoption neither waits on nor stops it.
func TestLateFinish_AgentGoneStillAdopts(t *testing.T) {
	for _, tc := range []struct {
		name       string
		commit     bool
		commitless bool
		want       string
	}{
		{"land", true, false, "landed "},
		{"commitless", false, true, "closed commitless"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runnerShapes(t, func(t *testing.T, headless bool) {
				p := parkZeroCommit(t, headless)
				p.agentGone()
				if tc.commit {
					commitAgentWork(t, p.wtPath)
				}
				p.report(t, p.finishedLate(tc.commitless))
				if reason := p.adopted(t); !strings.Contains(reason, tc.want) {
					t.Errorf("reason = %q, want %q", reason, tc.want)
				}
				if n := p.stops(); n != 0 {
					t.Errorf("runner stopped %d times, want none", n)
				}
			})
		})
	}
}

func TestLateFinish_FailedConditionStaysParked(t *testing.T) {
	for _, tc := range []struct {
		name   string
		commit bool
		mutate func(p zeroCommitParked) func(*schema.Ticket)
	}{
		{"status changed", true, func(p zeroCommitParked) func(*schema.Ticket) {
			return func(tk *schema.Ticket) { p.finishedLate(false)(tk); tk.Status = schema.StatusOpen }
		}},
		{"reclaimed", true, func(p zeroCommitParked) func(*schema.Ticket) {
			return func(tk *schema.Ticket) { p.finishedLate(false)(tk); tk.Status = schema.StatusClaimed }
		}},
		{"kind changed", true, func(p zeroCommitParked) func(*schema.Ticket) {
			return func(tk *schema.Ticket) { p.finishedLate(false)(tk); tk.ParkKind = schema.ParkKindBlockedPane }
		}},
		{"self-reported park", false, func(p zeroCommitParked) func(*schema.Ticket) {
			return func(tk *schema.Ticket) { p.finishedLate(true)(tk); tk.ParkKind = schema.ParkKindSelfReported }
		}},
		{"finished before park", true, func(p zeroCommitParked) func(*schema.Ticket) {
			return func(tk *schema.Ticket) { p.finishedLate(false)(tk); tk.FinishedAt = p.park.Time.Add(-time.Second) }
		}},
		{"no finished report", false, func(p zeroCommitParked) func(*schema.Ticket) {
			return func(tk *schema.Ticket) { tk.FinishedAt = p.park.Time.Add(time.Second); tk.Commitless = true }
		}},
		{"late commit without finished", true, func(p zeroCommitParked) func(*schema.Ticket) {
			return func(tk *schema.Ticket) { tk.FinishedAt = p.park.Time.Add(time.Second) }
		}},
		{"zero commits without commitless", false, func(p zeroCommitParked) func(*schema.Ticket) {
			return p.finishedLate(false)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runnerShapes(t, func(t *testing.T, headless bool) {
				p := parkZeroCommit(t, headless)
				// Every condition is checked before the agent is looked up; with
				// it gone a wrong adoption skips the settle wait and shows at once.
				p.agentGone()
				if tc.commit {
					commitAgentWork(t, p.wtPath)
				}
				p.report(t, tc.mutate(p))
				p.staysParked(t)
			})
		})
	}
}
