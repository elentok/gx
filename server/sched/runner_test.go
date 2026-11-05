package sched_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

func TestRunner_RefusesAClaimWhenTheFileChangedUnderTheIndexThenClaimsOnTheNextPass(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.DisableWatch = true // a missed watch event: only the poll catches the edit up
		c.PollInterval = time.Hour
		c.BudgetPollInterval = time.Hour // a budget poll also rescans, which would catch the index up
	})
	servertest.RegisterTurn(h)
	// Each call gets its own deadline: one shared context ran out under heavy load.
	call := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), 30*time.Second)
	}
	// A pass that finds herdr unprobed claims nothing, which would hide the refusal.
	servertest.WaitFor(t, 30*time.Second, "herdr to be probed", func() bool {
		ctx, cancel := call()
		defer cancel()
		snap, err := h.Client.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return !snap.HerdrUnavailable
	})

	path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	// The server rescans once right after start; let that finish so the edit merges after it.
	// Not a condition wait: nothing observable marks the end of that rescan, and a snapshot
	// that already lists the ticket does not (the edit then lands inside the rescan).
	time.Sleep(300 * time.Millisecond)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, "\nedited after the index saw it\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	addCtx, addCancel := call()
	defer addCancel()
	if res, err := h.Client.Queue.Add(addCtx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	servertest.WaitFor(t, 30*time.Second, "the claim-reread-mismatch verdict", func() bool {
		ctx, cancel := call()
		defer cancel()
		ex, err := h.Client.Reads.Explain(ctx, "proj:epic-a/01")
		if err != nil {
			t.Fatal(err)
		}
		return ex.Verdict == wire.VerdictClaimRereadMismatch
	})
	if got, _ := os.ReadFile(path); !strings.Contains(string(got), "status: open") {
		t.Fatalf("ticket was claimed on a stale index view:\n%s", got)
	}

	// A restart is the next pass over a fresh index: it sees the edit and claims.
	h.Restart(t)
	servertest.WaitFor(t, 30*time.Second, "the ticket to be claimed after the index caught up", func() bool {
		got := servertest.ReadFile(t, path)
		return strings.Contains(got, "status: claimed") || strings.Contains(got, "status: done")
	})
}

func TestRunner_BackfillsTheNextQueuedRootWhenASlotFrees(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-b", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.MaxAgents = 1
	})
	h.OnTurn(func(turn *servertest.Turn) {
		// Root A merges on main first, so root B needs work of its own to commit.
		testutil.WriteFile(t, turn.Cwd, filepath.Base(turn.Cwd)+".txt", "work")
		testutil.CommitAll(t, turn.Cwd, "agent work")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range []string{"proj:epic-a/01", "proj:epic-b/01"} {
		if res, err := h.Client.Queue.Add(ctx, addr, "claude"); err != nil || res.Refused {
			t.Fatalf("add %s: %+v, %v", addr, res, err)
		}
	}

	var seen []string
	completed := 0
	for ev := range evs {
		switch ev.Type {
		case wire.EventIterationStarted, wire.EventTicketDone, wire.EventRootCompleted:
			seen = append(seen, ev.Type+" "+ev.Address)
		case wire.EventIterationFailed, wire.EventIterationPaused:
			t.Fatalf("unexpected %s for %s", ev.Type, ev.Address)
		}
		if ev.Type == wire.EventRootCompleted {
			if completed++; completed == 2 {
				break
			}
		}
	}
	// With one slot, root B starts only after root A's ticket is done.
	if len(seen) != 6 || !strings.HasPrefix(seen[1], wire.EventTicketDone) {
		t.Errorf("events = %v, want start/done/completed per root, one root at a time", seen)
	}
}

func TestRunner_ClaimsAnotherTicketOfARootOnlyBelowThePerRootCap(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		perRoot int
		starts  bool
	}{{1, false}, {2, true}} {
		store, repo := t.TempDir(), testutil.TempRepo(t)
		servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
		servertest.SetProjectRepo(t, store, "proj", repo)
		h := servertest.StartWithStore(t, store, func(c *server.Config) {
			c.MaxAgentsPerRoot = tc.perRoot
			c.PollInterval = 50 * time.Millisecond
		})
		servertest.RegisterTurn(h)
		// One agent of the root is already running.
		h.Server.Registry().PutRun("proj:epic-a", runs.Run{Address: "proj:epic-a/00", Runner: "herdr"})
		// The no-start case waits out the whole timeout, so it stays short.
		timeout := time.Second
		if tc.starts {
			timeout = 20 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		snap, err := h.Client.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		evs, err := h.Client.Events(ctx, snap.Seq)
		if err != nil {
			t.Fatal(err)
		}
		if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
			t.Fatalf("add: %+v, %v", res, err)
		}
		started := false
		for ev := range evs {
			if ev.Type == wire.EventIterationStarted {
				started = true
				break
			}
		}
		if started != tc.starts {
			t.Errorf("per-root cap %d: ticket 01 started = %v, want %v", tc.perRoot, started, tc.starts)
		}
		cancel()
	}
}

// Seam: a ticket that already has a registry entry is not launched a second
// time; the claim moves on to the next frontier ticket of the root.
func TestRunner_SkipsAFrontierTicketThatIsAlreadyTracked(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.MaxAgentsPerRoot = 2
		c.PollInterval = 50 * time.Millisecond
	})
	servertest.RegisterTurn(h)
	h.Server.Registry().PutRun("proj:epic-a", runs.Run{Address: "proj:epic-a/01", Runner: "pty"})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	for ev := range evs {
		if ev.Type == wire.EventIterationStarted {
			if ev.Address != "proj:epic-a/02" {
				t.Fatalf("started %s, want proj:epic-a/02", ev.Address)
			}
			return
		}
	}
	t.Fatal("no iteration started")
}

// Seam A: a project's max-agents holds it to that many live agents while
// another project uses the free slots; editing the cap applies at the next
// decision, with no restart.
func TestRunner_ProjectCapHoldsItsProjectAndAnEditAppliesLive(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj-a", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj-a", "epic-b", "01", "first", "")
	servertest.WriteTicket(t, store, "proj-b", "epic-c", "01", "first", "")
	writeProject := func(project string, maxAgents int) {
		body := fmt.Sprintf(`{"name":%q,"repo":%q,"max-agents":%d}`, project, repo, maxAgents)
		if err := os.WriteFile(filepath.Join(store, project, "project.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeProject("proj-a", 1)
	servertest.SetProjectRepo(t, store, "proj-b", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.PollInterval = 50 * time.Millisecond
	})
	servertest.RegisterTurn(h)
	h.Server.Registry().PutRun("proj-a:epic-a", runs.Run{Address: "proj-a:epic-a/00", Runner: "herdr"}) // proj-a is at its cap
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range []string{"proj-a:epic-b/01", "proj-b:epic-c/01"} {
		if res, err := h.Client.Queue.Add(ctx, addr, "claude"); err != nil || res.Refused {
			t.Fatalf("add %s: %+v, %v", addr, res, err)
		}
	}
	nextStart := func() string {
		for ev := range evs {
			if ev.Type == wire.EventIterationStarted {
				return ev.Address
			}
		}
		t.Fatal("event stream ended before an iteration started")
		return ""
	}
	if got := nextStart(); got != "proj-b:epic-c/01" {
		t.Fatalf("first start = %s, want proj-b:epic-c/01 (proj-a is at its cap)", got)
	}
	writeProject("proj-a", 2)
	if got := nextStart(); got != "proj-a:epic-b/01" {
		t.Errorf("after raising the cap, start = %s, want proj-a:epic-b/01", got)
	}
}

func TestExplain_QueuedTicketBehindAFullLimitExplainsTheCapThenOneChangeStreamsWhenTheSlotFrees(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-b", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.MaxAgents = 1
	})
	// epic-a's turn is held until the test frees the slot.
	release := make(chan struct{})
	h.OnTurn(func(*servertest.Turn) { <-release })
	var once sync.Once
	free := func() { once.Do(func() { close(release) }) }
	defer free()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, addr := range []string{"proj:epic-a/01", "proj:epic-b/01"} {
		if res, err := h.Client.Queue.Add(ctx, addr, "claude"); err != nil || res.Refused {
			t.Fatalf("add %s: %+v, %v", addr, res, err)
		}
	}
	var snap wire.Snapshot
	// The claim reaches the index after the watch debounce, so the first row
	// reads claimed a moment after the cap shows up.
	servertest.WaitFor(t, 30*time.Second, "the first root claimed and the second at the cap", func() bool {
		var err error
		if snap, err = h.Client.Snapshot(ctx); err != nil {
			t.Fatal(err)
		}
		return len(snap.Pending) == 2 && snap.Pending[0].Verdict == "claimed" && snap.Pending[1].Verdict == wire.VerdictConcurrencyCap
	})
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}

	free()
	for ev := range evs {
		if ev.Type != wire.EventExplainVerdictChange || ev.Address != "proj:epic-b/01" {
			continue
		}
		ex, err := h.Client.Reads.Explain(ctx, ev.Address)
		if err != nil {
			t.Fatal(err)
		}
		if ex.Verdict == wire.VerdictConcurrencyCap {
			t.Errorf("verdict-change event but explain still says %+v", ex)
		}
		return
	}
	t.Fatal("no verdict-change event for the second root")
}

// Seam A: a ticket waiting on two full caps explains both, with counts.
func TestExplain_NamesEveryFullCapWithItsCount(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-b", "01", "first", "")
	body := fmt.Sprintf(`{"name":"proj","repo":%q,"max-agents":1}`, repo)
	if err := os.WriteFile(filepath.Join(store, "proj", "project.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.PollInterval = 50 * time.Millisecond
		c.MaxAgents = 1
	})
	servertest.RegisterTurn(h)
	h.Server.Registry().PutRun("proj:epic-a", runs.Run{Address: "proj:epic-a/00", Runner: "herdr"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-b/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	ex, err := h.Client.Reads.Explain(ctx, "proj:epic-b/01")
	if err != nil {
		t.Fatal(err)
	}
	if want := "project cap 1/1, global cap 1/1"; ex.Verdict != wire.VerdictConcurrencyCap || ex.Reason != want {
		t.Errorf("explain = %+v, want %s: %s", ex, wire.VerdictConcurrencyCap, want)
	}
}

// Seam A: the cap is global, so roots of different projects share it, and the
// queue is plain FIFO across projects.
func TestRunner_GlobalCapIsSharedAcrossProjectsInFIFOOrder(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj-a", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj-b", "epic-b", "01", "first", "")
	servertest.WriteTicket(t, store, "proj-a", "epic-c", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj-a", repo)
	servertest.SetProjectRepo(t, store, "proj-b", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.MaxAgents = 2
		// The fake's finish wait times out at once, so only real-time pacing
		// keeps the agents from exhausting their finish budget mid-test.
		c.Sleep = time.Sleep
	})
	// No OnTurn: the prompt starts no turn, so started agents keep their slot.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	order := []string{"proj-a:epic-a/01", "proj-b:epic-b/01", "proj-a:epic-c/01"}
	var last []wire.PendingRow
	for _, addr := range order {
		if res, err := h.Client.Queue.Add(ctx, addr, "claude"); err != nil || res.Refused {
			t.Fatalf("add %s: %+v, %v", addr, res, err)
		}
	}
	// A slot is taken a moment before its ticket reads "claimed", so the first
	// two verdicts may lag the third's cap; keep polling until they settle.
	servertest.WaitFor(t, 30*time.Second, "the first two claimed and the third at the cap", func() bool {
		snap, err := h.Client.Snapshot(ctx)
		if err != nil {
			t.Fatalf("snapshot: %v; last pending = %+v", err, last)
		}
		last = snap.Pending
		return len(snap.Pending) == 3 && snap.Pending[2].Verdict == wire.VerdictConcurrencyCap &&
			snap.Pending[0].Verdict == "claimed" && snap.Pending[1].Verdict == "claimed"
	})
	if runs := h.Server.Runs(); len(runs) != 2 {
		t.Errorf("runs = %+v, want 2 live agents", runs)
	}
}

func TestRunner_RestartPausesAHandleWhoseWorktreeIsGone(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, servertest.WithHerdr(servertest.HerdrIsTheSubject))
	registerLiveAgent(h)
	wtPath, ticketPath := leaveIteration(t, h, store, repo, "herdr")
	if err := os.RemoveAll(wtPath); err != nil {
		t.Fatal(err)
	}

	h.Restart(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	for ev := range evs {
		if ev.Type == wire.EventTicketPaused {
			break
		}
	}
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	assertPaused(t, tk, schema.PauseClassRepair, schema.PauseHandleMismatch)
	body, _ := os.ReadFile(ticketPath)
	if !strings.Contains(string(body), schema.PausedHeading) {
		t.Errorf("ticket has no Paused section:\n%s", body)
	}
	logged, _, err := runlog.ReadEvents(filepath.Join(store, "proj"), "epic-a")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, ev := range logged {
		if ev.Kind == string(events.HandleMismatch) {
			n++
		}
	}
	if n != 1 {
		t.Errorf("handle-mismatch events = %d, want 1 (%v)", n, logged)
	}
}

func TestRunner_APauseSendsOnePrefixedChatMessageNoMatterHowManyClientsWatch(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	chat := newSlackHook(t)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Chat = notify.ServerChatConfig{SlackWebhookURL: chat.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json"), FlushInterval: testFlushInterval}
	})
	servertest.RegisterTurn(h) // the agent finishes without committing, so the ticket pauses
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	started := chat.WaitStarted(t) // so the pause message never shares a post with it

	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var streams []<-chan wire.Event
	for range 2 { // two connected clients
		evs, err := h.Client.Events(ctx, snap.Seq)
		if err != nil {
			t.Fatal(err)
		}
		streams = append(streams, evs)
	}
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	for _, evs := range streams {
		for ev := range evs {
			if ev.Type == wire.EventIterationPaused {
				break
			}
		}
	}

	posts := chat.PostsSince(started)
	if len(posts) != 1 {
		t.Fatalf("chat sends = %d, want exactly 1: %v", len(posts), posts)
	}
	if !strings.Contains(posts[0], "*proj*") {
		t.Errorf("message lacks the project header: %s", posts[0])
	}
}
