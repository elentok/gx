package launchtest_test

import (
	"context"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
)

// promptHoldRunner holds the launch prompt (step 4) until the server's stop
// cancels it, as a runner waiting for an agent's ready signal does.
type promptHoldRunner struct {
	*runnerfake.Runner
	inPrompt chan struct{}
}

func (p *promptHoldRunner) PromptLaunch(ctx context.Context, _ agentrunner.Session, _ string, _ func(string)) error {
	close(p.inPrompt)
	<-ctx.Done()
	return ctx.Err()
}

// stopMidLaunch queues proj:epic-a/01, waits until entered is closed, stops the
// server and returns how long the stop took, with the repo and ticket path.
func stopMidLaunch(t *testing.T, entered <-chan struct{}, opts ...func(*server.Config)) (h *servertest.Harness, repo, ticketPath string, took time.Duration) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h = servertest.StartWithStore(t, store, opts...)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("launch never reached the step to stop in")
	}
	_ = filepath.WalkDir(store, func(p string, d fs.DirEntry, _ error) error {
		if d != nil && strings.HasPrefix(d.Name(), "01-") && strings.HasSuffix(d.Name(), ".md") {
			ticketPath = p
		}
		return nil
	})
	start := time.Now()
	if err := h.Stop(); err != nil {
		t.Fatal(err)
	}
	return h, repo, ticketPath, time.Since(start)
}

// assertRolledBack checks what a stop must leave: the ticket open, no
// worktree or branch, no registry entry, and no launch-failed pause.
func assertRolledBack(t *testing.T, h *servertest.Harness, repo, ticketPath string) {
	t.Helper()
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != schema.StatusOpen {
		t.Errorf("ticket status = %q, want open", tk.Status)
	}
	out, err := exec.Command("git", "-C", repo, "worktree", "list").CombinedOutput()
	if err != nil || strings.Count(strings.TrimSpace(string(out)), "\n") != 1 {
		t.Errorf("worktrees = %q (%v), want only the repo and the epic's feature worktree", out, err)
	}
	if out, _ := exec.Command("git", "-C", repo, "branch", "--list", "ralph-loop/*").CombinedOutput(); strings.TrimSpace(string(out)) != "" {
		t.Errorf("iteration branch left behind: %s", out)
	}
	if rs := readRuns(t, filepath.Join(h.StateDir, "runs.json")); len(rs) != 0 {
		t.Errorf("runs.json = %v, want empty", rs)
	}
}

func TestLaunchStop_RollsBackAnInstallInFlightWithoutWaitingItOut(t *testing.T) {
	t.Parallel()
	entered := make(chan struct{})
	var once atomic.Bool
	install := func(ctx context.Context, _, _ string) ([]string, error) {
		if once.CompareAndSwap(false, true) {
			close(entered)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(30 * time.Second):
			return nil, nil
		}
	}
	h, repo, ticketPath, took := stopMidLaunch(t, entered, func(c *server.Config) { c.InstallDeps = install })
	if took > 10*time.Second {
		t.Errorf("stop took %v, it waited out the install", took)
	}
	assertRolledBack(t, h, repo, ticketPath)
}

func TestLaunchStop_DuringThePromptStopsTheStartedAgent(t *testing.T) {
	t.Parallel()
	fake := runnerfake.NewRunner()
	r := &promptHoldRunner{Runner: fake, inPrompt: make(chan struct{})}
	h, repo, ticketPath, _ := stopMidLaunch(t, r.inPrompt, servertest.WithRunner(r))
	if left, _ := fake.List("epic-a"); len(left) != 0 {
		t.Errorf("sessions after stop = %v, want none", left)
	}
	assertRolledBack(t, h, repo, ticketPath)
}

func TestLaunchStop_PublishesNoLaunchFailure(t *testing.T) {
	t.Parallel()
	r := &promptHoldRunner{Runner: runnerfake.NewRunner(), inPrompt: make(chan struct{})}
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, servertest.WithRunner(r))
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
	if _, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil {
		t.Fatal(err)
	}
	<-r.inPrompt
	stopped := make(chan error, 1)
	go func() { stopped <- h.Stop() }()
	for ev := range evs {
		if ev.Type == wire.EventIterationLaunchFailed {
			t.Fatal("a stop published iteration-launch-failed")
		}
	}
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
}
