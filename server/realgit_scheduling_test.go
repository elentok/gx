package server_test

// Server-harness ports of ralphloop's run_realgit_subtickets / scheduling
// scenarios (seam A). The originals stay in ralphloop until cutover.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

const schedEpic = "epic"

// schedFixture is one project with one epic, a server over it, and an agent
// whose turns are recorded in order. turn runs after the agent's commit.
type schedFixture struct {
	store, repo string
	h           *servertest.Harness
	issues      string
	mu          sync.Mutex
	order       []string
}

func newSchedFixture(t *testing.T, tickets map[string]servertest.TicketOpts, agentTurn func(f *schedFixture, p servertest.Prompt, id string), cfgOpts ...func(*server.Config)) *schedFixture {
	t.Helper()
	f := &schedFixture{store: t.TempDir(), repo: testutil.TempRepo(t)}
	for id, opts := range tickets {
		servertest.WriteTicketWith(t, f.store, "proj", schedEpic, id, "t", opts)
	}
	servertest.SetProjectRepo(t, f.store, "proj", f.repo)
	f.issues = filepath.Join(f.store, "proj", schedEpic, "issues")
	f.h = servertest.StartWithStore(t, f.store, func(c *server.Config) {
		c.PollInterval = 50 * time.Millisecond
		c.MaxAgentsPerRoot = 1 // these scenarios pin the sequential order
		for _, o := range cfgOpts {
			o(c)
		}
	})
	t.Cleanup(func() {
		if t.Failed() {
			log, _ := os.ReadFile(server.LogPath(f.h.StateDir))
			t.Logf("server log:\n%s", log)
		}
	})
	f.h.RegisterLaunch(func(p servertest.Prompt) {
		_, id, _ := strings.Cut(p.Address, "/")
		f.mu.Lock()
		f.order = append(f.order, id)
		f.mu.Unlock()
		agentTurn(f, p, id)
	})
	return f
}

// commitWork is the agent's turn: one commit named after the ticket, in its worktree.
func commitWork(t *testing.T, p servertest.Prompt, name string) {
	t.Helper()
	testutil.WriteFile(t, p.Cwd, name+".txt", name)
	testutil.CommitAll(t, p.Cwd, "work "+name)
}

func (f *schedFixture) turns() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.order)
}

// queueEpic queues the epic and returns the event stream from before the add.
func (f *schedFixture) queueEpic(ctx context.Context, t *testing.T, firstID string) <-chan server.Event {
	t.Helper()
	snap, err := f.h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := f.h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := f.h.Client.QueueAdd(ctx, "proj:"+schedEpic+"/"+firstID, "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	return evs
}

func (f *schedFixture) ticketFile(t *testing.T, id string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(f.issues, id+"-*.md"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("ticket %s files = %v, %v", id, matches, err)
	}
	return matches[0]
}

func (f *schedFixture) git(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", f.repo}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// assertLanded checks each ticket is done, its commit is on the feature branch
// and no iteration branch is left behind.
func (f *schedFixture) assertLanded(t *testing.T, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if body := readFile(t, f.ticketFile(t, id)); !strings.Contains(body, "status: done") {
			t.Errorf("ticket %s not done:\n%s", id, body)
		}
		if got := f.git(t, "show", schedEpic+":"+id+".txt"); got != id {
			t.Errorf("feature branch %s:%s.txt = %q", schedEpic, id, got)
		}
	}
	if left := strings.TrimSpace(f.git(t, "branch", "--list", "ralph-loop/*")); left != "" {
		t.Errorf("iteration branches left behind:\n%s", left)
	}
}

func TestSchedulingPort_TicketCreatesSubticketsAndTheRootWaitsForThem(t *testing.T) {
	var once sync.Once
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p servertest.Prompt, id string) {
		commitWork(t, p, id)
		if id != "01" {
			return
		}
		once.Do(func() {
			for _, child := range []string{"01a", "01b"} {
				servertest.WriteTicketWith(t, f.store, "proj", schedEpic, child, "child", servertest.TicketOpts{Parent: "01"})
			}
		})
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")

	order := f.turns()
	if len(order) != 3 || order[0] != "01" {
		t.Errorf("turns = %v, want 01 first then each child exactly once", order)
	}
	f.assertLanded(t, "01", "01a", "01b")
}

func TestSchedulingPort_CodeReviewTicketCreatesSubtickets(t *testing.T) {
	var once sync.Once
	f := newSchedFixture(t, map[string]servertest.TicketOpts{
		"01": {},
		"02": {Type: "code-review"},
	}, func(f *schedFixture, p servertest.Prompt, id string) {
		commitWork(t, p, id)
		if id != "02" {
			return
		}
		once.Do(func() {
			for _, child := range []string{"02a", "02b"} {
				servertest.WriteTicketWith(t, f.store, "proj", schedEpic, child, "child", servertest.TicketOpts{Parent: "02"})
			}
		})
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")

	order := f.turns()
	if len(order) != 4 || order[0] != "01" || order[1] != "02" {
		t.Errorf("turns = %v, want 01, then the review 02, then its findings", order)
	}
	f.assertLanded(t, "01", "02", "02a", "02b")
}

// The per-root cap is pinned to 1 here, so this pins the sequential order and the single landing per ticket.
func TestSchedulingPort_ABlocksBAndCAndTheyLandInOrder(t *testing.T) {
	f := newSchedFixture(t, map[string]servertest.TicketOpts{
		"01": {},
		"02": {BlockedBy: []string{"01"}},
		"03": {BlockedBy: []string{"01"}},
	}, func(_ *schedFixture, p servertest.Prompt, id string) { commitWork(t, p, id) })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")

	if order := f.turns(); len(order) != 3 || order[0] != "01" {
		t.Errorf("turns = %v, want 01 before 02 and 03", order)
	}
	f.assertLanded(t, "01", "02", "03")
	if n := strings.Count(f.git(t, "log", "--format=%s", schedEpic), "work "); n != 3 {
		t.Errorf("feature branch has %d work commits, want exactly 3", n)
	}
}

func TestSchedulingPort_ParkThenResumeReusesBranchAndLandsBothCommitsOnce(t *testing.T) {
	var mu sync.Mutex
	turn := 0
	var ticketPath string
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p servertest.Prompt, id string) {
		mu.Lock()
		turn++
		cur := turn
		mu.Unlock()
		if cur == 1 {
			commitWork(t, p, "pre")
			// The agent reports a question for a person.
			body := readFile(t, ticketPath)
			body = strings.Replace(body, "status: claimed\n", "status: claimed\niteration_status: needs-answer\n", 1)
			if err := os.WriteFile(ticketPath, []byte(body), 0o644); err != nil {
				t.Error(err)
			}
			return
		}
		commitWork(t, p, "post")
	})
	ticketPath = f.ticketFile(t, "01")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventIterationParked, "proj:epic/01")

	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != schema.StatusNeedsAnswer {
		t.Fatalf("status after park = %q, want needs-answer", tk.Status)
	}
	if left := strings.TrimSpace(f.git(t, "branch", "--list", "ralph-loop/*")); left == "" {
		t.Error("iteration branch gone after park, want it kept for the resume")
	}
	if got := strings.TrimSpace(f.git(t, "log", "--format=%s", schedEpic)); strings.Contains(got, "work pre") {
		t.Errorf("a parked iteration landed: %s", got)
	}

	// A person answers and the ticket is picked back up.
	if err := ralphloop.UnparkTicket(ticketPath, time.Now()); err != nil {
		t.Fatal(err)
	}
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")

	for _, name := range []string{"pre", "post"} {
		if got := f.git(t, "show", schedEpic+":"+name+".txt"); got != name {
			t.Errorf("feature branch lacks %s.txt (got %q)", name, got)
		}
	}
	if n := strings.Count(f.git(t, "log", "--format=%s", schedEpic), "work "); n != 2 {
		t.Errorf("feature branch has %d work commits, want pre and post exactly once", n)
	}
	if left := strings.TrimSpace(f.git(t, "branch", "--list", "ralph-loop/*")); left != "" {
		t.Errorf("iteration branch left after landing:\n%s", left)
	}
}
