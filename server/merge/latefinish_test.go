package merge_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func TestLateFinish_MergesLateCommit(t *testing.T) {
	t.Parallel()
	runnerShapes(t, func(t *testing.T, headless bool) {
		p := pauseZeroCommit(t, headless)
		commitAgentWork(t, p.WtPath)
		p.Report(t, p.FinishedLate(false))
		reason := p.Adopted(t)
		sha := ralphloop.LatestMergedSHA(filepath.Join(p.Store, "proj"), "epic-a", "01")
		if sha == "" || !strings.Contains(reason, "merged "+sha) || !strings.Contains(reason, "paused "+pausedAt(p)) {
			t.Errorf("reason = %q, want paused %s and merged %q", reason, pausedAt(p), sha)
		}
		if p.Stops() == 0 {
			t.Error("live agent not stopped after merging")
		}
	})
}

func TestLateFinish_ClosesQuestionAsAdopted(t *testing.T) {
	t.Parallel()
	p := pauseZeroCommit(t, true)
	evs, err := tickets.ReadLog(p.TicketPath)
	if err != nil {
		t.Fatal(err)
	}
	q := tickets.OpenQuestion(evs)
	if q == nil {
		t.Fatal("zero-commit pause left no open question")
	}
	p.Report(t, p.FinishedLate(true))
	p.Adopted(t)
	evs, _ = tickets.ReadLog(p.TicketPath)
	last := evs[len(evs)-1]
	if last.Type != tickets.EventPauseClosed || last.Question != q.Seq || last.Reason != "adopted" {
		t.Errorf("last event = %+v, want pause.closed{question: %d, reason: adopted}", last, q.Seq)
	}
	if last.Source != tickets.SourceGx || last.How != tickets.HowAuto {
		t.Errorf("last event = %+v, want source gx, how auto", last)
	}
}

func TestLateFinish_ClosesCommitless(t *testing.T) {
	t.Parallel()
	runnerShapes(t, func(t *testing.T, headless bool) {
		p := pauseZeroCommit(t, headless)
		p.Report(t, p.FinishedLate(true))
		if reason := p.Adopted(t); !strings.Contains(reason, "closed commitless") {
			t.Errorf("reason = %q, want closed commitless", reason)
		}
		// 01 is the epic's last ticket, so the adoption completes the epic.
		epicMD := filepath.Join(p.Store, "proj/epic-a/ticket.md")
		waitFor(t, func() bool {
			b, err := os.ReadFile(epicMD)
			return err == nil && strings.Contains(string(b), "merged_at:") && strings.Contains(string(b), "completed_at:")
		})
	})
}

// With the agent gone the adoption neither waits on nor stops it.
func TestLateFinish_AgentGoneStillAdopts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		commit     bool
		commitless bool
		want       string
	}{
		{"merge", true, false, "merged "},
		{"commitless", false, true, "closed commitless"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runnerShapes(t, func(t *testing.T, headless bool) {
				p := pauseZeroCommit(t, headless)
				p.AgentGone()
				if tc.commit {
					commitAgentWork(t, p.WtPath)
				}
				p.Report(t, p.FinishedLate(tc.commitless))
				if reason := p.Adopted(t); !strings.Contains(reason, tc.want) {
					t.Errorf("reason = %q, want %q", reason, tc.want)
				}
				if n := p.Stops(); n != 0 {
					t.Errorf("runner stopped %d times, want none", n)
				}
			})
		})
	}
}

// resume and relaunch adopt a late finish before acting, launch nothing, and
// cancel only the investigation that has not started.
func TestLateFinish_ResumeAndRelaunchAdoptFirst(t *testing.T) {
	t.Parallel()
	for _, verb := range []string{"resume", "relaunch"} {
		t.Run(verb, func(t *testing.T) {
			t.Parallel()
			runnerShapes(t, func(t *testing.T, headless bool) {
				// No poll may pick up the investigations before the verb runs.
				p := pauseZeroCommit(t, headless, func(c *server.Config) { c.PollInterval = time.Hour })
				commitAgentWork(t, p.WtPath)
				// No rescan ping: the verb itself must run the check. The late finish
				// lands before the children, so a loop wake-up under load cannot claim
				// the unstarted one first.
				if err := schema.UpdateTicket(p.TicketPath, p.FinishedLate(false)); err != nil {
					t.Fatal(err)
				}
				servertest.WriteTicketWith(t, p.Store, "proj", "epic-a", "02", "unstarted", servertest.TicketOpts{Type: "investigate", Parent: "01"})
				servertest.WriteTicketWith(t, p.Store, "proj", "epic-a", "03", "running", servertest.TicketOpts{Type: "investigate", Parent: "01", Status: "claimed"})
				ctx := context.Background()
				if verb == "resume" {
					res, err := p.H.Client.Repair(ctx, "resume", wire.RepairRequest{Address: "proj:epic-a/01"})
					if err != nil || res.Refused || !strings.Contains(string(res.Data), `"adopted"`) {
						t.Fatalf("resume = %+v (data %s), %v; want adopted", res, res.Data, err)
					}
				} else if res, err := p.H.Client.Tickets.Relaunch(ctx, "proj:epic-a/01", false); err != nil || res.Refused {
					t.Fatalf("relaunch = %+v, %v", res, err)
				}
				if tk, err := schema.ParseTicket(p.TicketPath); err != nil || tk.Status != schema.StatusDone {
					t.Fatalf("after %s: status = %v, %v; want done", verb, tk.Status, err)
				}
				if reason := p.Adopted(t); !strings.Contains(reason, "merged ") {
					t.Errorf("reason = %q, want merged", reason)
				}
				unstarted := filepath.Join(p.Store, "proj/epic-a/issues/02-unstarted.md")
				if tk, err := schema.ParseTicket(unstarted); err != nil || tk.Status != schema.StatusCancelled {
					t.Errorf("unstarted investigation status = %v, %v; want cancelled", tk.Status, err)
				}
				if body, err := os.ReadFile(unstarted); err != nil || !strings.Contains(string(body), "## Resolution\n\nlate finish adopted") {
					t.Errorf("unstarted investigation body lacks the resolution (%v):\n%s", err, body)
				}
				if tk, err := schema.ParseTicket(filepath.Join(p.Store, "proj/epic-a/issues/03-running.md")); err != nil || tk.Status == schema.StatusCancelled {
					t.Errorf("running investigation status = %v, %v; want left alone", tk.Status, err)
				}
				// A launch would reclaim it.
				time.Sleep(200 * time.Millisecond) // negative wait: a few 50ms polls
				if tk, err := schema.ParseTicket(p.TicketPath); err != nil || tk.Status != schema.StatusDone {
					t.Errorf("status = %v, %v; want still done", tk.Status, err)
				}
			})
		})
	}
}

func TestLateFinish_FailedConditionStaysPaused(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		commit bool
		mutate func(p zeroCommitPaused) func(*schema.Ticket)
	}{
		{"status changed", true, func(p zeroCommitPaused) func(*schema.Ticket) {
			// Not open: an open ticket is a launch candidate, and the loop would run it to done.
			return func(tk *schema.Ticket) { p.FinishedLate(false)(tk); tk.Status = schema.StatusCancelled }
		}},
		{"reclaimed", true, func(p zeroCommitPaused) func(*schema.Ticket) {
			return func(tk *schema.Ticket) { p.FinishedLate(false)(tk); tk.Status = schema.StatusClaimed }
		}},
		{"kind changed", true, func(p zeroCommitPaused) func(*schema.Ticket) {
			return func(tk *schema.Ticket) { p.FinishedLate(false)(tk); tk.PauseReason = schema.PauseBlockedPane }
		}},
		{"self-reported pause", false, func(p zeroCommitPaused) func(*schema.Ticket) {
			return func(tk *schema.Ticket) { p.FinishedLate(true)(tk); tk.PauseReason = schema.PauseSelfReported }
		}},
		{"finished before pause", true, func(p zeroCommitPaused) func(*schema.Ticket) {
			return func(tk *schema.Ticket) {
				p.FinishedLate(false)(tk)
				tk.IterationFinishedAt = p.Pause.Time.Add(-time.Second)
			}
		}},
		{"no finished report", false, func(p zeroCommitPaused) func(*schema.Ticket) {
			return func(tk *schema.Ticket) { tk.IterationFinishedAt = p.Pause.Time.Add(time.Second); tk.Commitless = true }
		}},
		{"late commit without finished", true, func(p zeroCommitPaused) func(*schema.Ticket) {
			return func(tk *schema.Ticket) { tk.IterationFinishedAt = p.Pause.Time.Add(time.Second) }
		}},
		{"zero commits without commitless", false, func(p zeroCommitPaused) func(*schema.Ticket) {
			return p.FinishedLate(false)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// The conditions are runner-independent, so one shape covers them.
			p := pauseZeroCommit(t, true)
			// Every condition is checked before the agent is looked up; with
			// it gone a wrong adoption skips the settle wait and shows at once.
			p.AgentGone()
			if tc.commit {
				commitAgentWork(t, p.WtPath)
			}
			p.Report(t, tc.mutate(p))
			p.StaysPaused(t)
		})
	}
}
