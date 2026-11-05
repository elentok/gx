package sched_test

// Server-harness ports of ralphloop's run_realgit_subtickets / scheduling
// scenarios (seam A).

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/schema"
)

const schedEpic = servertest.SchedEpic

var commitWork = servertest.CommitWork

// schedFixture adds the root-only helpers (other files' methods) to the shared
// servertest.SchedFixture. Its callbacks receive the wrapper.
type schedFixture struct {
	*servertest.SchedFixture
}

func newSchedFixture(t *testing.T, tickets map[string]servertest.TicketOpts, agentTurn func(f *schedFixture, p *servertest.Turn, id string), cfgOpts ...func(*server.Config)) *schedFixture {
	t.Helper()
	f := &schedFixture{}
	var inner func(*servertest.SchedFixture, *servertest.Turn, string)
	if agentTurn != nil {
		inner = func(_ *servertest.SchedFixture, p *servertest.Turn, id string) { agentTurn(f, p, id) }
	}
	f.SchedFixture = servertest.NewSchedFixture(t, tickets, inner, cfgOpts...)
	return f
}

func TestSchedulingPort_TicketCreatesSubticketsAndTheRootWaitsForThem(t *testing.T) {
	t.Parallel()
	var once sync.Once
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
		if id != "01" {
			return
		}
		once.Do(func() {
			for _, child := range []string{"01a", "01b"} {
				servertest.WriteTicketWith(t, f.Store, "proj", schedEpic, child, "child", servertest.TicketOpts{Parent: "01"})
			}
		})
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootCompleted, "")

	order := f.Turns()
	if len(order) != 3 || order[0] != "01" {
		t.Errorf("turns = %v, want 01 first then each child exactly once", order)
	}
	f.AssertMerged(t, "01", "01a", "01b")
}

func TestSchedulingPort_CodeReviewTicketCreatesSubtickets(t *testing.T) {
	t.Parallel()
	var once sync.Once
	f := newSchedFixture(t, map[string]servertest.TicketOpts{
		"01": {},
		"02": {Type: "code-review"},
	}, func(f *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
		if id != "02" {
			return
		}
		once.Do(func() {
			for _, child := range []string{"02a", "02b"} {
				servertest.WriteTicketWith(t, f.Store, "proj", schedEpic, child, "child", servertest.TicketOpts{Parent: "02"})
			}
		})
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootCompleted, "")

	order := f.Turns()
	if len(order) != 4 || order[0] != "01" || order[1] != "02" {
		t.Errorf("turns = %v, want 01, then the review 02, then its findings", order)
	}
	f.AssertMerged(t, "01", "02", "02a", "02b")
}

// The per-root cap is pinned to 1 here, so this pins the sequential order and the single merging per ticket.
func TestSchedulingPort_ABlocksBAndCAndTheyMergeInOrder(t *testing.T) {
	t.Parallel()
	f := newSchedFixture(t, map[string]servertest.TicketOpts{
		"01": {},
		"02": {BlockedBy: []string{"01"}},
		"03": {BlockedBy: []string{"01"}},
	}, func(_ *schedFixture, p *servertest.Turn, id string) { commitWork(t, p, id) })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootCompleted, "")

	if order := f.Turns(); len(order) != 3 || order[0] != "01" {
		t.Errorf("turns = %v, want 01 before 02 and 03", order)
	}
	f.AssertMerged(t, "01", "02", "03")
	if n := strings.Count(f.Git(t, "log", "--format=%s", schedEpic), "work "); n != 3 {
		t.Errorf("feature branch has %d work commits, want exactly 3", n)
	}
}

func TestSchedulingPort_PauseThenResumeReusesBranchAndMergesBothCommitsOnce(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	turn := 0
	var ticketPath string
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p *servertest.Turn, id string) {
		mu.Lock()
		turn++
		cur := turn
		mu.Unlock()
		if cur == 1 {
			commitWork(t, p, "pre")
			// The agent reports a question for a person.
			body := servertest.ReadFile(t, ticketPath)
			body = strings.Replace(body, "status: claimed\n", "status: claimed\niteration_status: asking\n", 1)
			if err := os.WriteFile(ticketPath, []byte(body), 0o644); err != nil {
				t.Error(err)
			}
			return
		}
		commitWork(t, p, "post")
	})
	ticketPath = f.TicketFile(t, "01")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventIterationPaused, "proj:epic/01")

	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	assertPaused(t, tk, schema.PauseClassAnswer, "")
	if t.Failed() {
		t.FailNow()
	}
	if left := strings.TrimSpace(f.Git(t, "branch", "--list", "ralph-loop/*")); left == "" {
		t.Error("iteration branch gone after pause, want it kept for the resume")
	}
	if got := strings.TrimSpace(f.Git(t, "log", "--format=%s", schedEpic)); strings.Contains(got, "work pre") {
		t.Errorf("a paused iteration merged: %s", got)
	}

	// A person answers and the ticket is picked back up.
	// The PTY agent waits idle for the answer, so the answer is what resumes it.
	state := pauseWithQuestion(t, ticketPath, optionsBlock)
	res, err := f.H.Client.Tickets.Answer(ctx, wire.QueueRequest{Address: "proj:" + schedEpic + "/01", State: state, Choice: "a"})
	if err != nil || res.Refused {
		t.Fatalf("answer = %+v, %v", res, err)
	}
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootCompleted, "")

	for _, name := range []string{"pre", "post"} {
		if got := f.Git(t, "show", schedEpic+":"+name+".txt"); got != name {
			t.Errorf("feature branch lacks %s.txt (got %q)", name, got)
		}
	}
	if n := strings.Count(f.Git(t, "log", "--format=%s", schedEpic), "work "); n != 2 {
		t.Errorf("feature branch has %d work commits, want pre and post exactly once", n)
	}
	if left := strings.TrimSpace(f.Git(t, "branch", "--list", "ralph-loop/*")); left != "" {
		t.Errorf("iteration branch left after merging:\n%s", left)
	}
}
