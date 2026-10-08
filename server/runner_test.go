package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// registerLaunch answers the herdr commands a launch makes and records the
// agent start and prompt argv.
func registerLaunch(h *servertest.Harness) (start, prompt *[]string, cwd *string) {
	start, prompt, cwd = new([]string), new([]string), new(string)
	agent := map[string]any{"agent": map[string]any{"pane_id": "p1", "agent_status": "idle"}}
	h.Herdr.Register("workspace", "create", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"workspace": map[string]any{"workspace_id": "w1"}}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("tab", "create", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		for i, a := range argv {
			if a == "--cwd" && i+1 < len(argv) {
				*cwd = argv[i+1]
			}
		}
		return map[string]any{"tab": map[string]any{"tab_id": "t1"}, "root_pane": map[string]any{"pane_id": "p1"}}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("tab", "close", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "start", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		*start = argv
		return agent, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "wait", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return agent, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "prompt", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		*prompt = argv
		return agent, herdrfake.Identities{}, nil
	})
	return start, prompt, cwd
}

func TestRunner_ClaimsWriteTheFileBeforeTheEventAndLaunchWithTheChosenAgent(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Orchestrator = config.OrchestratorServer })
	start, prompt, _ := registerLaunch(h)
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
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "codex"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}

	claimedOnDisk := false
	for ev := range evs {
		if ev.Type == server.EventTicketClaimed {
			data, err := os.ReadFile(filepath.Join(store, "proj", "epic-a", "issues", "01-first.md"))
			if err != nil {
				t.Fatal(err)
			}
			claimedOnDisk = strings.Contains(string(data), "status: claimed")
			if ev.Address != "proj:epic-a/01" {
				t.Errorf("claim event address = %q", ev.Address)
			}
		}
		if ev.Type == server.EventIterationStarted {
			break
		}
	}
	if !claimedOnDisk {
		t.Error("ticket file did not say claimed when the claim event streamed")
	}
	if !strings.Contains(strings.Join(*start, " "), "--kind codex") {
		t.Errorf("agent start = %v, want kind codex", *start)
	}
	if !strings.Contains(strings.Join(*prompt, " "), "$gx-implement proj:epic-a/01") {
		t.Errorf("agent prompt = %v, want the codex skill prompt", *prompt)
	}
	if runs := h.Server.Runs(); len(runs) != 1 || runs[0].Address != "proj:epic-a/01" || runs[0].Agent != "codex" {
		t.Errorf("runs = %+v", runs)
	}
}

// gx-implement asks for commits, which a code-review ticket never makes.
func TestRunner_CodeReviewTicketLaunchesUnderTheCodeReviewSkill(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "review", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	path := filepath.Join(store, "proj", "epic-a", "issues", "01-review.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), "type: implement", "type: code-review", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Orchestrator = config.OrchestratorServer })
	_, prompt, _ := registerLaunch(h)
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
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "codex"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	for ev := range evs {
		if ev.Type == server.EventIterationStarted {
			break
		}
	}
	if got := strings.Join(*prompt, " "); !strings.Contains(got, "$gx-code-review proj:epic-a/01") {
		t.Errorf("agent prompt = %v, want the gx-code-review skill", got)
	}
}

func TestRunner_LaunchSkillFollowsTicketType(t *testing.T) {
	for _, tc := range []struct{ typ, want string }{
		{"prompt", "$gx-one-off proj:epic-a/01"},
		{"implement", "$gx-implement proj:epic-a/01"},
	} {
		t.Run(tc.typ, func(t *testing.T) {
			store, repo := t.TempDir(), testutil.TempRepo(t)
			servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "first", servertest.TicketOpts{Type: tc.typ})
			servertest.SetProjectRepo(t, store, "proj", repo)
			h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Orchestrator = config.OrchestratorServer })
			_, prompt, _ := registerLaunch(h)
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
			if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "codex"); err != nil || res.Refused {
				t.Fatalf("add: %+v, %v", res, err)
			}
			for ev := range evs {
				if ev.Type == server.EventIterationStarted {
					break
				}
			}
			if got := strings.Join(*prompt, " "); !strings.Contains(got, tc.want) {
				t.Errorf("agent prompt = %q, want it to contain %q", got, tc.want)
			}
		})
	}
}

func TestRunner_RefusesAClaimWhenTheFileChangedUnderTheIndexThenClaimsOnTheNextPass(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.DisableWatch = true // a missed watch event: only the poll catches the edit up
		c.PollInterval = time.Hour
	})
	registerLaunch(h)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// A pass that finds herdr unprobed claims nothing, which would hide the refusal.
	for {
		snap, err := h.Client.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !snap.HerdrUnavailable {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	// The server rescans once right after start; let that finish so the edit lands after it.
	time.Sleep(300 * time.Millisecond)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, "\nedited after the index saw it\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "codex"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	for {
		ex, err := h.Client.Explain(ctx, "proj:epic-a/01")
		if err != nil {
			t.Fatal(err)
		}
		if ex.Verdict == server.VerdictClaimRereadMismatch {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got, _ := os.ReadFile(path); !strings.Contains(string(got), "status: open") {
		t.Fatalf("ticket was claimed on a stale index view:\n%s", got)
	}

	// A restart is the next pass over a fresh index: it sees the edit and claims.
	h.Restart(t)
	for !strings.Contains(readFile(t, path), "status: claimed") && !strings.Contains(readFile(t, path), "status: done") {
		if ctx.Err() != nil {
			t.Fatal("ticket was not claimed after the index caught up")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRunner_LandsTheIterationAndClosesTheRoot(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Orchestrator = config.OrchestratorServer })
	_, _, cwd := registerLaunch(h)
	// The agent's turn: commit a file in its own worktree.
	h.Herdr.Register("agent", "prompt", func(_ *herdrfake.State, _ []string) (any, herdrfake.Identities, error) {
		testutil.WriteFile(t, *cwd, "agent.txt", "work")
		testutil.CommitAll(t, *cwd, "agent work")
		return map[string]any{"agent": map[string]any{"pane_id": "p1", "agent_status": "idle"}}, herdrfake.Identities{}, nil
	})
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
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}

	var seen []string
	for ev := range evs {
		switch ev.Type {
		case server.EventTicketClaimed, server.EventIterationStarted, server.EventTicketDone, server.EventRootCompleted,
			server.EventIterationFailed, server.EventIterationParked:
			seen = append(seen, ev.Type)
		}
		if ev.Type == server.EventRootCompleted || ev.Type == server.EventIterationFailed {
			break
		}
	}
	want := []string{server.EventTicketClaimed, server.EventIterationStarted, server.EventTicketDone, server.EventRootCompleted}
	if strings.Join(seen, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", seen, want)
	}
	data, err := os.ReadFile(filepath.Join(store, "proj", "epic-a", "issues", "01-first.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "status: done") {
		t.Errorf("ticket file is not done:\n%s", data)
	}
	out, err := exec.Command("git", "-C", repo, "show", "epic-a:agent.txt").CombinedOutput()
	if err != nil || string(out) != "work" {
		t.Errorf("feature branch lacks the agent's commit: %q, %v", out, err)
	}
}

func TestRunner_AFinishThatErrorsParksTheTicketNeedsRepairAndFreesTheRoot(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Orchestrator = config.OrchestratorServer })
	_, _, cwd := registerLaunch(h)
	// The agent's turn wrecks its own worktree, so the finish cannot read it.
	h.Herdr.Register("agent", "prompt", func(_ *herdrfake.State, _ []string) (any, herdrfake.Identities, error) {
		if err := os.RemoveAll(*cwd); err != nil {
			t.Error(err)
		}
		return map[string]any{"agent": map[string]any{"pane_id": "p1", "agent_status": "idle"}}, herdrfake.Identities{}, nil
	})
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
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	for ev := range evs {
		if ev.Type == server.EventIterationFailed {
			break
		}
	}
	tk, err := schema.ParseTicket(filepath.Join(store, "proj", "epic-a", "issues", "01-first.md"))
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != schema.StatusNeedsRepair || tk.ParkKind != schema.ParkKind(events.IterationError) {
		t.Errorf("ticket = (%q, %q), want needs-repair/iteration-error", tk.Status, tk.ParkKind)
	}
	if runs := h.Server.Runs(); len(runs) != 0 {
		t.Errorf("runs = %+v, want the root freed", runs)
	}
}

// A claim that parks on an ambiguous base launches nothing, so claimNext must
// not count it against the concurrency limit. The frontier never offers such a
// ticket (it shares the blocking rule with base derivation), so the claim is
// driven directly.
func TestRunner_AmbiguousBaseParksWithoutLaunching(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "03", "third", servertest.TicketOpts{BlockedBy: []string{"01", "02"}})
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Orchestrator = config.OrchestratorServer })
	registerLaunch(h)

	launched, err := h.Server.ClaimTicket("proj:epic-a/03", "claude")
	if err != nil {
		t.Fatal(err)
	}
	if launched {
		t.Error("claim reported launched, want parked with no slot used")
	}
	if runs := h.Server.Runs(); len(runs) != 0 {
		t.Errorf("runs = %+v, want none", runs)
	}
	body, err := os.ReadFile(filepath.Join(store, "proj", "epic-a", "issues", "03-third.md"))
	if err != nil || !strings.Contains(string(body), "status: needs-answer") {
		t.Errorf("ticket 03 (%v):\n%s\nwant status: needs-answer", err, body)
	}
}

func TestRunner_BackfillsTheNextQueuedRootWhenASlotFrees(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-b", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.MaxAgents = 1
	})
	_, _, cwd := registerLaunch(h)
	h.Herdr.Register("agent", "prompt", func(_ *herdrfake.State, _ []string) (any, herdrfake.Identities, error) {
		// Root A lands on main first, so root B needs work of its own to commit.
		testutil.WriteFile(t, *cwd, filepath.Base(*cwd)+".txt", "work")
		testutil.CommitAll(t, *cwd, "agent work")
		return map[string]any{"agent": map[string]any{"pane_id": "p1", "agent_status": "idle"}}, herdrfake.Identities{}, nil
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
		if res, err := h.Client.QueueAdd(ctx, addr, "claude"); err != nil || res.Refused {
			t.Fatalf("add %s: %+v, %v", addr, res, err)
		}
	}

	var seen []string
	completed := 0
	for ev := range evs {
		switch ev.Type {
		case server.EventIterationStarted, server.EventTicketDone, server.EventRootCompleted:
			seen = append(seen, ev.Type+" "+ev.Address)
		case server.EventIterationFailed, server.EventIterationParked:
			t.Fatalf("unexpected %s for %s", ev.Type, ev.Address)
		}
		if ev.Type == server.EventRootCompleted {
			if completed++; completed == 2 {
				break
			}
		}
	}
	// With one slot, root B starts only after root A's ticket is done.
	if len(seen) != 6 || !strings.HasPrefix(seen[1], server.EventTicketDone) {
		t.Errorf("events = %v, want start/done/completed per root, one root at a time", seen)
	}
}

func TestRunner_ClaimsAnotherTicketOfARootOnlyBelowThePerRootCap(t *testing.T) {
	for _, tc := range []struct {
		perRoot int
		starts  bool
	}{{1, false}, {2, true}} {
		store, repo := t.TempDir(), testutil.TempRepo(t)
		servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
		servertest.SetProjectRepo(t, store, "proj", repo)
		h := servertest.StartWithStore(t, store, func(c *server.Config) {
			c.Orchestrator = config.OrchestratorServer
			c.MaxAgentsPerRoot = tc.perRoot
			c.PollInterval = 50 * time.Millisecond
		})
		registerLaunch(h)
		// One agent of the root is already running.
		h.Server.PutRun("proj:epic-a", server.Run{Address: "proj:epic-a/00"})
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		snap, err := h.Client.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		evs, err := h.Client.Events(ctx, snap.Seq)
		if err != nil {
			t.Fatal(err)
		}
		if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
			t.Fatalf("add: %+v, %v", res, err)
		}
		started := false
		for ev := range evs {
			if ev.Type == server.EventIterationStarted {
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

// Seam A: a project's max-agents holds it to that many live agents while
// another project uses the free slots; editing the cap applies at the next
// decision, with no restart.
func TestRunner_ProjectCapHoldsItsProjectAndAnEditAppliesLive(t *testing.T) {
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
		c.Orchestrator = config.OrchestratorServer
		c.PollInterval = 50 * time.Millisecond
	})
	registerLaunch(h)
	h.Server.PutRun("proj-a:epic-a", server.Run{Address: "proj-a:epic-a/00"}) // proj-a is at its cap
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
		if res, err := h.Client.QueueAdd(ctx, addr, "claude"); err != nil || res.Refused {
			t.Fatalf("add %s: %+v, %v", addr, res, err)
		}
	}
	nextStart := func() string {
		for ev := range evs {
			if ev.Type == server.EventIterationStarted {
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
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-b", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.MaxAgents = 1
	})
	registerLaunch(h)
	// The first wait is the launch's; the second is epic-a's iteration, held
	// until the test frees the slot.
	release := make(chan struct{})
	var waits atomic.Int32
	idle := map[string]any{"agent": map[string]any{"pane_id": "p1", "agent_status": "idle"}}
	h.Herdr.Register("agent", "wait", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		if waits.Add(1) == 2 {
			<-release
		}
		return idle, herdrfake.Identities{}, nil
	})
	var once sync.Once
	free := func() { once.Do(func() { close(release) }) }
	defer free()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, addr := range []string{"proj:epic-a/01", "proj:epic-b/01"} {
		if res, err := h.Client.QueueAdd(ctx, addr, "claude"); err != nil || res.Refused {
			t.Fatalf("add %s: %+v, %v", addr, res, err)
		}
	}
	var snap server.Snapshot
	for {
		var err error
		if snap, err = h.Client.Snapshot(ctx); err != nil {
			t.Fatal(err)
		}
		if len(snap.Pending) == 2 && snap.Pending[1].Verdict == server.VerdictConcurrencyCap {
			break
		}
		if ctx.Err() != nil {
			t.Fatalf("pending = %+v, want the second root at the cap", snap.Pending)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := snap.Pending[0].Verdict; got != "claimed" {
		t.Errorf("first pending verdict = %q, want claimed", got)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}

	free()
	for ev := range evs {
		if ev.Type != server.EventExplainVerdictChange || ev.Address != "proj:epic-b/01" {
			continue
		}
		ex, err := h.Client.Explain(ctx, ev.Address)
		if err != nil {
			t.Fatal(err)
		}
		if ex.Verdict == server.VerdictConcurrencyCap {
			t.Errorf("verdict-change event but explain still says %+v", ex)
		}
		return
	}
	t.Fatal("no verdict-change event for the second root")
}

// Seam A: a ticket waiting on two full caps explains both, with counts.
func TestExplain_NamesEveryFullCapWithItsCount(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-b", "01", "first", "")
	body := fmt.Sprintf(`{"name":"proj","repo":%q,"max-agents":1}`, repo)
	if err := os.WriteFile(filepath.Join(store, "proj", "project.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.PollInterval = 50 * time.Millisecond
		c.MaxAgents = 1
	})
	registerLaunch(h)
	h.Server.PutRun("proj:epic-a", server.Run{Address: "proj:epic-a/00"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-b/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	ex, err := h.Client.Explain(ctx, "proj:epic-b/01")
	if err != nil {
		t.Fatal(err)
	}
	if want := "project cap 1/1, global cap 1/1"; ex.Verdict != server.VerdictConcurrencyCap || ex.Reason != want {
		t.Errorf("explain = %+v, want %s: %s", ex, server.VerdictConcurrencyCap, want)
	}
}

// Seam A: the cap is global, so roots of different projects share it, and the
// queue is plain FIFO across projects.
func TestRunner_GlobalCapIsSharedAcrossProjectsInFIFOOrder(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj-a", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj-b", "epic-b", "01", "first", "")
	servertest.WriteTicket(t, store, "proj-a", "epic-c", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj-a", repo)
	servertest.SetProjectRepo(t, store, "proj-b", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.MaxAgents = 2
	})
	registerLaunch(h)
	// Every iteration wait blocks until released, so started agents keep their slot.
	release := make(chan struct{})
	var once sync.Once
	free := func() { once.Do(func() { close(release) }) }
	defer free()
	idle := map[string]any{"agent": map[string]any{"pane_id": "p1", "agent_status": "idle"}}
	var waits atomic.Int32
	h.Herdr.Register("agent", "wait", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		if waits.Add(1) > 3 { // the three launches' waits come first
			<-release
		}
		return idle, herdrfake.Identities{}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	order := []string{"proj-a:epic-a/01", "proj-b:epic-b/01", "proj-a:epic-c/01"}
	for _, addr := range order {
		if res, err := h.Client.QueueAdd(ctx, addr, "claude"); err != nil || res.Refused {
			t.Fatalf("add %s: %+v, %v", addr, res, err)
		}
	}
	for {
		snap, err := h.Client.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(snap.Pending) == 3 && snap.Pending[2].Verdict == server.VerdictConcurrencyCap {
			if a, b := snap.Pending[0].Verdict, snap.Pending[1].Verdict; a != "claimed" || b != "claimed" {
				t.Errorf("first two verdicts = %q, %q, want both claimed", a, b)
			}
			break
		}
		if ctx.Err() != nil {
			t.Fatalf("pending = %+v, want the third root at the cap", snap.Pending)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if runs := h.Server.Runs(); len(runs) != 2 {
		t.Errorf("runs = %+v, want 2 live agents", runs)
	}
}

// leaveIteration stages what a server that died mid-iteration leaves behind: a
// claimed ticket, its worktree with the agent's commit, and the persisted
// handle. It returns the worktree path and the ticket file.
func leaveIteration(t *testing.T, h *servertest.Harness, store, repo string) (string, string) {
	t.Helper()
	epics, err := tickets.Load(filepath.Join(store, "proj"))
	if err != nil {
		t.Fatal(err)
	}
	tk := epics[0].Tickets[0]
	if err := ralphloop.Claim(tk.Path); err != nil {
		t.Fatal(err)
	}
	wt, err := ralphloop.PrepareIteration(ralphloop.DefaultDeps(), ralphloop.OneIteration{
		RepoDir: repo, Epic: "epic-a", ScratchDir: store, Agent: ralphloop.AgentClaude, Ticket: tk,
	})
	if err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(t, wt.Path, "agent.txt", "work")
	testutil.CommitAll(t, wt.Path, "agent work")
	handle, err := json.Marshal([]map[string]string{{
		"address": "proj:epic-a/01", "agent": "claude", "pane": "p1", "tab": "t1", "root": "proj:epic-a",
		"repo": repo, "workspace": "w1", "base": wt.Base(), "ticket_path": tk.Path,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.StateDir, "runs.json"), handle, 0o600); err != nil {
		t.Fatal(err)
	}
	return wt.Path, tk.Path
}

func registerLiveAgent(h *servertest.Harness) {
	idle := map[string]any{"agent": map[string]any{"pane_id": "p1", "tab_id": "t1", "agent_status": "idle"}}
	for _, verb := range []string{"get", "wait"} {
		h.Herdr.Register("agent", verb, func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
			return idle, herdrfake.Identities{}, nil
		})
	}
	h.Herdr.Register("tab", "close", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{}, herdrfake.Identities{}, nil
	})
}

func TestRunner_RestartParksAHandleWhoseWorktreeIsGone(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Orchestrator = config.OrchestratorServer })
	registerLiveAgent(h)
	wtPath, ticketPath := leaveIteration(t, h, store, repo)
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
		if ev.Type == server.EventTicketParked {
			break
		}
	}
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != schema.StatusNeedsRepair || tk.ParkKind != schema.ParkKind(events.HandleMismatch) {
		t.Errorf("status=%q park_kind=%q, want needs-repair/handle-mismatch", tk.Status, tk.ParkKind)
	}
	body, _ := os.ReadFile(ticketPath)
	if !strings.Contains(string(body), "## Needs Repair") {
		t.Errorf("ticket has no Needs Repair section:\n%s", body)
	}
	logged, _, err := ralphloop.ReadEvents(filepath.Join(store, "proj"), "epic-a")
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

func TestRunner_RestartReclaimsTheLiveIterationAndItFinishes(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Orchestrator = config.OrchestratorServer })
	registerLiveAgent(h)
	leaveIteration(t, h, store, repo)

	h.Restart(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	for ev := range evs {
		switch ev.Type {
		case server.EventReclaimed, server.EventIterationStarted, server.EventTicketDone, server.EventRootCompleted,
			server.EventIterationFailed, server.EventIterationParked:
			seen = append(seen, ev.Type)
		}
		if ev.Type == server.EventRootCompleted || ev.Type == server.EventIterationFailed {
			break
		}
	}
	want := []string{server.EventReclaimed, server.EventTicketDone, server.EventRootCompleted}
	if strings.Join(seen, ",") != strings.Join(want, ",") {
		log, _ := os.ReadFile(server.LogPath(h.StateDir))
		t.Fatalf("events = %v, want %v\nserver log:\n%s", seen, want, log)
	}
	// The land writes under the project, where the manual land and the
	// tickets live, never under a phantom <store>/<epic>.
	logged, _, err := ralphloop.ReadEvents(filepath.Join(store, "proj"), "epic-a")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(logged, func(ev ralphloop.Event) bool { return ev.Type == string(events.CherryPicked) }) {
		t.Errorf("no cherry-picked event in the project's run log: %v", logged)
	}
	if _, err := os.Stat(filepath.Join(store, "epic-a")); !os.IsNotExist(err) {
		t.Errorf("land wrote under the store root: stat %s: %v", filepath.Join(store, "epic-a"), err)
	}
}

func TestRunner_AParkSendsOnePrefixedChatMessageNoMatterHowManyClientsWatch(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	var mu sync.Mutex
	var bodies []string
	chat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), "server started") { // the start notice is not a park message
			return
		}
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
	}))
	defer chat.Close()
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.Chat = ralphloop.ServerChatConfig{SlackWebhookURL: chat.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")}
	})
	registerLaunch(h) // the agent finishes without committing, so the ticket parks
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var streams []<-chan server.Event
	for range 2 { // two connected clients
		evs, err := h.Client.Events(ctx, snap.Seq)
		if err != nil {
			t.Fatal(err)
		}
		streams = append(streams, evs)
	}
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	for _, evs := range streams {
		for ev := range evs {
			if ev.Type == server.EventIterationParked {
				break
			}
		}
	}

	waitFor := func(n int) {
		t.Helper()
		for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
			mu.Lock()
			got := len(bodies)
			mu.Unlock()
			if got >= n {
				return
			}
		}
	}
	waitFor(1)
	time.Sleep(time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 {
		t.Fatalf("chat sends = %d, want exactly 1: %v", len(bodies), bodies)
	}
	if !strings.Contains(bodies[0], "*proj*") {
		t.Errorf("message lacks the project header: %s", bodies[0])
	}
}
