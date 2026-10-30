package servertest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/tickets/schema"
)

func TestMain(m *testing.M) {
	RunSessionProcesses()
	herdrfake.RunHelperProcess()
	os.Exit(m.Run())
}

func startPty(t *testing.T, store, repo string, opts ...func(*server.Config)) *Harness {
	t.Helper()
	WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	SetProjectRepo(t, store, "proj", repo)
	return StartWithStore(t, store, opts...)
}

func ptyOf(t *testing.T, h *Harness, project string) *nativerunner.Pty {
	t.Helper()
	p, ok := h.cfg.RunnerFor("pty", project).(*ptyRunner)
	if !ok {
		t.Fatalf("RunnerFor(pty, %s) = %T, want the harness's *ptyRunner", project, h.cfg.RunnerFor("pty", project))
	}
	return p.Pty
}

func TestPty_OnTurnCommitsInTheWorktreeAndTheTicketMerges(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	h := startPty(t, store, repo)
	turns := make(chan Turn, 4)
	h.OnTurn(func(turn *Turn) {
		testutil.WriteFile(t, turn.Cwd, "agent.txt", "work")
		testutil.CommitAll(t, turn.Cwd, "agent work")
		turns <- *turn
	})
	if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}

	var turn Turn
	select {
	case turn = <-turns:
	case <-time.After(30 * time.Second):
		t.Fatal("OnTurn never ran")
	}
	ticket := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	WaitFor(t, 30*time.Second, "ticket done", func() bool {
		body, _ := os.ReadFile(ticket)
		return strings.Contains(string(body), "status: done")
	})

	if turn.Address != "proj:epic-a/01" {
		t.Errorf("Address = %q, want proj:epic-a/01", turn.Address)
	}
	if turn.Cwd == "" || !strings.Contains(turn.Cwd, "epic-a") {
		t.Errorf("Cwd = %q, want the ticket's worktree", turn.Cwd)
	}
	if !strings.Contains(turn.Text, "proj:epic-a/01") {
		t.Errorf("Text = %q, want the launch prompt naming the ticket", turn.Text)
	}
	out, err := exec.Command("git", "-C", repo, "show", "epic-a:agent.txt").CombinedOutput()
	if err != nil || string(out) != "work" {
		t.Errorf("feature branch agent.txt = %q (%v), want the turn's commit merged", out, err)
	}
}

func TestPty_TheRunRecordsThePtyRunner(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	h := startPty(t, store, repo)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	h.OnTurn(func(*Turn) {
		entered <- struct{}{}
		<-release
	})
	if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	select {
	case <-entered:
	case <-time.After(30 * time.Second):
		t.Fatal("OnTurn never ran")
	}

	runs := h.Server.Runs()
	if len(runs) != 1 || runs[0].Runner != "pty" {
		t.Errorf("runs = %+v, want one run on the pty runner", runs)
	}
}

func TestDefault_NoRunnerOptionRunsOnPtyWithOneStartAndNoHerdrCall(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	h := startPty(t, store, repo)
	h.OnTurn(func(*Turn) {})
	if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	WaitFor(t, 30*time.Second, "the launch", func() bool { return len(h.Starts()) == 1 })

	if runs := h.Server.Runs(); len(runs) != 1 || runs[0].Runner != "pty" {
		t.Errorf("runs = %+v, want one run on the pty runner", runs)
	}
	if h.Herdr != nil {
		t.Errorf("h.Herdr = %v, want nil: the default makes no herdr call", h.Herdr)
	}
	got := h.Starts()
	if got[0].Kind != "claude" || got[0].Cwd == "" || got[0].Label == "" {
		t.Errorf("start = %+v, want label, cwd and kind claude", got[0])
	}
}

func TestDefault_WithHerdrRecordsOneStartPerLaunch(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	h := startPty(t, store, repo, WithHerdr(HerdrIsTheSubject))
	h.OnTurn(func(*Turn) {})
	if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	WaitFor(t, 30*time.Second, "the launch", func() bool { return len(h.Starts()) == 1 })

	got := h.Starts()
	if got[0].Kind != "claude" || got[0].Cwd == "" || got[0].Label == "" {
		t.Errorf("start = %+v, want label, cwd and kind claude", got[0])
	}
}

// queueWithTurn queues the one ticket and returns once the hook has run.
func queueWithTurn(t *testing.T, h *Harness, hook func(*Turn)) {
	t.Helper()
	ran := make(chan struct{}, 1)
	h.OnTurn(func(turn *Turn) {
		hook(turn)
		ran <- struct{}{}
	})
	if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	select {
	case <-ran:
	case <-time.After(30 * time.Second):
		t.Fatal("OnTurn never ran")
	}
}

func liveIterations(t *testing.T, h *Harness) int {
	t.Helper()
	its, err := h.Client.Iterations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return len(its)
}

func TestPty_TurnStallLeavesTheRunWorkingWithNoTurnEnd(t *testing.T) {
	t.Parallel()
	h := startPty(t, t.TempDir(), testutil.TempRepo(t))
	queueWithTurn(t, h, func(turn *Turn) { turn.Stall() })
	WaitFor(t, 30*time.Second, "the live iteration", func() bool { return liveIterations(t, h) == 1 })

	time.Sleep(10 * ptyPollInterval) // negative wait: a wrong reap would come within a few polls

	if n := liveIterations(t, h); n != 1 {
		t.Errorf("live iterations = %d, want the stalled run still live", n)
	}
	if runs := h.Server.Runs(); len(runs) != 1 {
		t.Errorf("runs = %+v, want the one stalled run", runs)
	}
	if body, _ := os.ReadFile(filepath.Join(h.TicketStore, "proj", "epic-a", "issues", "01-first.md")); !strings.Contains(string(body), "status: claimed") {
		t.Errorf("ticket = %s, want it still claimed", body)
	}
}

func TestPty_TurnBlockShowsAPendingPermissionWithTheReason(t *testing.T) {
	t.Parallel()
	h := startPty(t, t.TempDir(), testutil.TempRepo(t))
	queueWithTurn(t, h, func(turn *Turn) { turn.Block("rg needle") })

	var card *server.PermissionCard
	WaitFor(t, 30*time.Second, "a pending permission", func() bool {
		d, err := h.Client.Detail(context.Background(), "proj:epic-a/01")
		card = d.Permission
		return err == nil && card != nil
	})

	if !strings.Contains(card.Description, "rg needle") {
		t.Errorf("permission = %+v, want the reason in its description", card)
	}
}

func TestPty_TurnExitEndsTheSession(t *testing.T) {
	t.Parallel()
	h := startPty(t, t.TempDir(), testutil.TempRepo(t))
	queueWithTurn(t, h, func(turn *Turn) { turn.Exit(3) })

	WaitFor(t, 30*time.Second, "the agent gone from the server", func() bool {
		return len(h.Starts()) == 1 && liveIterations(t, h) == 0
	})
}

// failRecorder is a testing.TB whose Errorf records instead of failing.
type failRecorder struct {
	testing.TB
	mu     sync.Mutex
	errors []string
}

func (r *failRecorder) Errorf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func TestHerdr_TurnControlFailsTheTestNamingTheControl(t *testing.T) {
	t.Parallel()
	for name, control := range map[string]func(*Turn){
		"Stall":      func(turn *Turn) { turn.Stall() },
		"Block":      func(turn *Turn) { turn.Block("why") },
		"Exit":       func(turn *Turn) { turn.Exit(1) },
		"Hold":       func(turn *Turn) { turn.Hold() },
		"Transcript": func(turn *Turn) { turn.Transcript("hi") },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := startPty(t, t.TempDir(), testutil.TempRepo(t), WithHerdr(HerdrIsTheSubject))
			rec := &failRecorder{TB: t}
			h.tb = rec
			queueWithTurn(t, h, control)
			WaitFor(t, 30*time.Second, "the failure report", func() bool {
				rec.mu.Lock()
				defer rec.mu.Unlock()
				return len(rec.errors) > 0
			})

			rec.mu.Lock()
			defer rec.mu.Unlock()
			if len(rec.errors) != 1 || !strings.Contains(rec.errors[0], "Turn."+name) || !strings.Contains(rec.errors[0], "PTY") {
				t.Errorf("errors = %q, want one naming Turn.%s and PTY", rec.errors, name)
			}
		})
	}
}

func TestHerdr_HarnessHooksFailTheTestNamingTheHook(t *testing.T) {
	t.Parallel()
	h := startPty(t, t.TempDir(), testutil.TempRepo(t), WithHerdr(HerdrIsTheSubject))
	rec := &failRecorder{TB: t}
	h.tb = rec

	h.FailNextStart(errors.New("boom"))
	h.Stops()
	h.BlockStops()

	for i, name := range []string{"FailNextStart", "Stops", "BlockStops"} {
		if len(rec.errors) != 3 || !strings.Contains(rec.errors[i], "Harness."+name) || !strings.Contains(rec.errors[i], "PTY") {
			t.Fatalf("errors = %q, want one each naming Harness.%s and PTY", rec.errors, name)
		}
	}
}

func TestPty_HoldKeepsTheRunWorkingUntilReleased(t *testing.T) {
	t.Parallel()
	h := startPty(t, t.TempDir(), testutil.TempRepo(t))
	var release func()
	queueWithTurn(t, h, func(turn *Turn) { release = turn.Hold() })
	WaitFor(t, 30*time.Second, "the live iteration", func() bool { return liveIterations(t, h) == 1 })

	time.Sleep(10 * ptyPollInterval) // negative wait: a wrong reap would come within a few polls
	if n := liveIterations(t, h); n != 1 {
		t.Fatalf("live iterations = %d, want the held run still live", n)
	}

	release()
	WaitFor(t, 30*time.Second, "the turn to end", func() bool { return liveIterations(t, h) == 0 })
}

func TestPty_FailNextStartPausesTheTicketAndFailsOnlyOneStart(t *testing.T) {
	t.Parallel()
	h := startPty(t, t.TempDir(), testutil.TempRepo(t))
	h.FailNextStart(errors.New("boom"))
	if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}

	WaitFor(t, 30*time.Second, "the ticket to pause", func() bool {
		body, _ := os.ReadFile(filepath.Join(h.TicketStore, "proj", "epic-a", "issues", "01-first.md"))
		return strings.Contains(string(body), "status: paused")
	})
	if starts := h.Starts(); len(starts) != 0 {
		t.Errorf("starts = %+v, want none: the failed start launched nothing", starts)
	}
	if h.failStart.Load() != nil {
		t.Error("the failure is still armed, want it spent by the one start")
	}
}

func TestPty_BlockStopsCountsAStopAndHoldsItUntilReleased(t *testing.T) {
	t.Parallel()
	h := startPty(t, t.TempDir(), testutil.TempRepo(t))
	release := h.BlockStops()
	h.OnTurn(func(turn *Turn) {
		testutil.WriteFile(t, turn.Cwd, "agent.txt", "work")
		testutil.CommitAll(t, turn.Cwd, "agent work")
	})
	if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}

	WaitFor(t, 30*time.Second, "the stop to be counted", func() bool { return h.Stops() == 1 })
	// A killed agent leaves its pid file, so count the processes still alive.
	alive := func() (n int) {
		files, _ := filepath.Glob(filepath.Join(h.Env("CLAUDE_CONFIG_DIR"), "sessions", "*.json"))
		for _, f := range files {
			var pf agentfake.PidFile
			body, _ := os.ReadFile(f)
			if json.Unmarshal(body, &pf) == nil && pf.Pid > 0 && syscall.Kill(pf.Pid, 0) == nil {
				n++
			}
		}
		return n
	}
	time.Sleep(10 * ptyPollInterval) // negative wait: a wrongly killed agent would be gone within a few polls
	if n := alive(); n != 1 {
		t.Fatalf("live agent processes = %d, want the one the blocked stop holds alive", n)
	}

	release()
	WaitFor(t, 30*time.Second, "the agent process to end", func() bool { return alive() == 0 })
	if n := h.Stops(); n != 1 {
		t.Errorf("stops = %d, want 1", n)
	}
}

func TestPty_TranscriptIsWhatTheServerReadsForTheSession(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	h := startPty(t, store, repo, func(c *server.Config) {
		c.Recovery = recovery.Default()
		for i := range c.Recovery.Entries {
			c.Recovery.Entries[i].Enabled = true
		}
	})
	h.OnTurn(func(turn *Turn) {
		turn.Transcript("The feature is already implemented on the feature branch.")
	})
	if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}

	// R3 matches the agent's last words, so only a transcript the server found
	// under the session's id can propose it.
	WaitFor(t, 30*time.Second, "R3 to match on the transcript", func() bool {
		files, _ := filepath.Glob(filepath.Join(store, "proj", "epic-a", "issues", "*.md"))
		for _, f := range files {
			if body, _ := os.ReadFile(f); strings.Contains(string(body), "R3") {
				return true
			}
		}
		return false
	})
}

func TestPty_EachProjectGetsItsOwnRunnerRoot(t *testing.T) {
	t.Parallel()
	h := startPty(t, t.TempDir(), testutil.TempRepo(t))

	a, b := ptyOf(t, h, "a"), ptyOf(t, h, "b")
	if a == b || a.Root == b.Root {
		t.Errorf("roots = %q and %q, want two runners with different roots", a.Root, b.Root)
	}
	if want := nativerunner.AgentsRoot(h.StateDir, "a"); a.Root != want {
		t.Errorf("Root = %q, want %q", a.Root, want)
	}
	if ptyOf(t, h, "a") != a {
		t.Error("a project must keep one Pty")
	}
}

func TestPty_UsesTheHarnessProcessTable(t *testing.T) {
	t.Parallel()
	h := startPty(t, t.TempDir(), testutil.TempRepo(t))

	if got := ptyOf(t, h, "a").Procs; got == nil || got != NewProcessTable() {
		t.Errorf("Procs = %T, want the harness's process table", got)
	}
}

// commitWork is an agent that committed a file in its worktree.
func commitWork(t *testing.T) func(string) {
	return func(wtPath string) {
		testutil.WriteFile(t, wtPath, "agent.txt", "work")
		testutil.CommitAll(t, wtPath, "agent work")
	}
}

func TestPty_LeavePtyRunLiveReattachesAfterRestart(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	h := startPty(t, store, repo)
	h.LeavePtyRun(t, store, repo, Target{"proj", "epic-a", "01"}, true, commitWork(t))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h.Restart(t)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	WaitForEvent(ctx, t, evs, server.EventReclaimed, "proj:epic-a/01")

	if runs := h.Server.Runs(); len(runs) != 1 || runs[0].Runner != "pty" || runs[0].Session.Label == "" {
		t.Errorf("runs = %+v, want the live run reattached on pty", runs)
	}
	if got := len(h.Starts()); got != 0 {
		t.Errorf("Starts = %d, want the staged launch not counted", got)
	}
}

func TestPty_LeavePtyRunDeadReclaimsAsDead(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	h := startPty(t, store, repo)
	_, ticketPath := h.LeavePtyRun(t, store, repo, Target{"proj", "epic-a", "01"}, false, commitWork(t))

	h.Restart(t)
	WaitFor(t, 30*time.Second, "the dead run to pause the ticket for repair", func() bool {
		tk, err := schema.ParseTicket(ticketPath)
		if err != nil {
			t.Fatal(err)
		}
		return tk.Status == schema.StatusPaused && tk.PausedClass() == schema.PauseClassRepair
	})
	body, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), nativerunner.ReasonAgentDied) {
		t.Errorf("ticket lacks %q:\n%s", nativerunner.ReasonAgentDied, body)
	}
	if runs := h.Server.Runs(); len(runs) != 0 {
		t.Errorf("runs = %+v, want the dead run dropped", runs)
	}
}

func TestPty_RestartBuildsFreshRunnersAndReclaimsTheLiveRun(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	h := startPty(t, store, repo)
	// No OnTurn: the slash prompt starts no turn, so the agent stays live and
	// Stop does not wait on a hook.
	if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	WaitFor(t, 30*time.Second, "the agent to read its prompt", func() bool {
		inputs, _ := filepath.Glob(filepath.Join(h.Env("HOME"), ".claude", "agentfake", "*.in"))
		for _, in := range inputs {
			if body, _ := os.ReadFile(in); strings.Contains(string(body), "epic-a/01") {
				return true
			}
		}
		return false
	})
	before := ptyOf(t, h, "proj")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h.Restart(t)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	WaitForEvent(ctx, t, evs, server.EventReclaimed, "proj:epic-a/01")

	if ptyOf(t, h, "proj") == before {
		t.Error("Restart kept the old runner, want a fresh one")
	}
	if runs := h.Server.Runs(); len(runs) != 1 || runs[0].Runner != "pty" {
		t.Errorf("runs = %+v, want the live run reclaimed on pty", runs)
	}
}

// holderPID is the pid of label's holder process, read from its agent dir.
func holderPID(t *testing.T, h *Harness, project, label string) int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(ptyOf(t, h, project).Root, label, nativerunner.MetaFile))
	if err != nil {
		t.Fatal(err)
	}
	var meta nativerunner.Meta
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	return meta.HolderPID
}

func TestPty_ResumeAfterRestartStartsNoSecondClaude(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	h := startPty(t, store, repo)
	h.LeavePtyRun(t, store, repo, Target{"proj", "epic-a", "01"}, true, commitWork(t))
	const label = "epic-a-iter-01"
	before := holderPID(t, h, "proj", label)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h.Restart(t)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	WaitForEvent(ctx, t, evs, server.EventReclaimed, "proj:epic-a/01")

	if after := holderPID(t, h, "proj", label); after != before {
		t.Errorf("holder pid %d -> %d, want the same holder adopted", before, after)
	}
	if got := len(h.Starts()); got != 0 {
		t.Errorf("Starts = %d, want no second claude started", got)
	}
	files, _ := filepath.Glob(filepath.Join(h.Env("CLAUDE_CONFIG_DIR"), "sessions", "*.json"))
	if len(files) != 1 {
		t.Errorf("agent session files = %d, want the one claude", len(files))
	}
}

func TestPty_StaleHolderIsClosedAndTheLaunchRetried(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	h := startPty(t, store, repo)
	h.OnTurn(func(*Turn) {})
	const label = "epic-a-iter-01"
	// An idle holder of the launch's label in another directory is not adopted.
	holder, err := ptyOf(t, h, "proj").Start(agentrunner.StartOptions{
		Label: label, Epic: "epic-a", Cwd: t.TempDir(), Kind: agentrunner.Kind("claude"),
	})
	if err != nil {
		t.Fatal(err)
	}
	WaitFor(t, 30*time.Second, "the holder to be idle", func() bool {
		st, err := ptyOf(t, h, "proj").Status(holder)
		return err == nil && st.Waiting()
	})
	if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}

	WaitFor(t, 30*time.Second, "the launch to hold a session", func() bool {
		runs := h.Server.Runs()
		return len(runs) == 1 && runs[0].Session.ID != ""
	})
	logged, _, err := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
	if err != nil {
		t.Fatal(err)
	}
	var closed []ralphloop.Event
	for _, ev := range logged {
		if ev.Type == string(events.StalePaneClosed) {
			closed = append(closed, ev)
		}
	}
	if len(closed) != 1 || closed[0].Pane != holder.ID {
		t.Errorf("stale-pane-closed events = %+v, want one for %s", closed, holder.ID)
	}
	if runs := h.Server.Runs(); runs[0].Session.SessionID == holder.SessionID {
		t.Errorf("run session = %+v, want a fresh session", runs[0].Session)
	}
}
