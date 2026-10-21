package server_test

import (
	"context"
	"os"
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
	"github.com/elentok/gx/tickets"
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
		tk.IterationFinishedAt = p.park.Time.Add(time.Second)
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

func TestLateFinish_ClosesQuestionAsAdopted(t *testing.T) {
	p := parkZeroCommit(t, true)
	evs, err := tickets.ReadLog(p.ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	q := tickets.OpenQuestion(evs)
	if q == nil {
		t.Fatal("zero-commit park left no open question")
	}
	p.report(t, p.finishedLate(true))
	p.adopted(t)
	evs, _ = tickets.ReadLog(p.ticketPath)
	last := evs[len(evs)-1]
	if last.Type != tickets.EventParkClosed || last.Question != q.Seq || last.Reason != "adopted" {
		t.Errorf("last event = %+v, want park.closed{question: %d, reason: adopted}", last, q.Seq)
	}
}

func TestLateFinish_ClosesCommitless(t *testing.T) {
	runnerShapes(t, func(t *testing.T, headless bool) {
		p := parkZeroCommit(t, headless)
		p.report(t, p.finishedLate(true))
		if reason := p.adopted(t); !strings.Contains(reason, "closed commitless") {
			t.Errorf("reason = %q, want closed commitless", reason)
		}
		// 01 is the epic's last ticket, so the adoption completes the epic.
		epicMD := filepath.Join(p.store, "proj/epic-a/ticket.md")
		waitFor(t, func() bool {
			b, err := os.ReadFile(epicMD)
			return err == nil && strings.Contains(string(b), "merged_at:") && strings.Contains(string(b), "completed_at:")
		})
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

// unpark and relaunch adopt a late finish before acting, launch nothing, and
// cancel only the investigation that has not started.
func TestLateFinish_UnparkAndRelaunchAdoptFirst(t *testing.T) {
	for _, verb := range []string{"unpark", "relaunch"} {
		t.Run(verb, func(t *testing.T) {
			runnerShapes(t, func(t *testing.T, headless bool) {
				p := parkZeroCommit(t, headless)
				servertest.WriteTicketWith(t, p.store, "proj", "epic-a", "02", "unstarted", servertest.TicketOpts{Type: "investigate", Parent: "01"})
				servertest.WriteTicketWith(t, p.store, "proj", "epic-a", "03", "running", servertest.TicketOpts{Type: "investigate", Parent: "01", Status: "claimed"})
				commitAgentWork(t, p.wtPath)
				// No rescan ping: the verb itself must run the check.
				if err := schema.UpdateTicket(p.ticketPath, p.finishedLate(false)); err != nil {
					t.Fatal(err)
				}
				ctx := context.Background()
				if verb == "unpark" {
					res, err := p.h.Client.Repair(ctx, "unpark", server.RepairRequest{Address: "proj:epic-a/01"})
					if err != nil || res.Refused || !strings.Contains(string(res.Data), `"adopted"`) {
						t.Fatalf("unpark = %+v (data %s), %v; want adopted", res, res.Data, err)
					}
				} else if res, err := p.h.Client.TicketRelaunch(ctx, "proj:epic-a/01", false); err != nil || res.Refused {
					t.Fatalf("relaunch = %+v, %v", res, err)
				}
				if tk, err := schema.ParseTicket(p.ticketPath); err != nil || tk.Status != schema.StatusDone {
					t.Fatalf("after %s: status = %v, %v; want done", verb, tk.Status, err)
				}
				if reason := p.adopted(t); !strings.Contains(reason, "landed ") {
					t.Errorf("reason = %q, want landed", reason)
				}
				unstarted := filepath.Join(p.store, "proj/epic-a/issues/02-unstarted.md")
				if tk, err := schema.ParseTicket(unstarted); err != nil || tk.Status != schema.StatusCancelled {
					t.Errorf("unstarted investigation status = %v, %v; want cancelled", tk.Status, err)
				}
				if body, err := os.ReadFile(unstarted); err != nil || !strings.Contains(string(body), "## Resolution\n\nlate finish adopted") {
					t.Errorf("unstarted investigation body lacks the resolution (%v):\n%s", err, body)
				}
				if tk, err := schema.ParseTicket(filepath.Join(p.store, "proj/epic-a/issues/03-running.md")); err != nil || tk.Status == schema.StatusCancelled {
					t.Errorf("running investigation status = %v, %v; want left alone", tk.Status, err)
				}
				// A launch would reclaim it.
				time.Sleep(300 * time.Millisecond)
				if tk, err := schema.ParseTicket(p.ticketPath); err != nil || tk.Status != schema.StatusDone {
					t.Errorf("status = %v, %v; want still done", tk.Status, err)
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
			return func(tk *schema.Ticket) {
				p.finishedLate(false)(tk)
				tk.IterationFinishedAt = p.park.Time.Add(-time.Second)
			}
		}},
		{"no finished report", false, func(p zeroCommitParked) func(*schema.Ticket) {
			return func(tk *schema.Ticket) { tk.IterationFinishedAt = p.park.Time.Add(time.Second); tk.Commitless = true }
		}},
		{"late commit without finished", true, func(p zeroCommitParked) func(*schema.Ticket) {
			return func(tk *schema.Ticket) { tk.IterationFinishedAt = p.park.Time.Add(time.Second) }
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
