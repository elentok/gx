package servertest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
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
