package server_test

// Server-harness ports of ralphloop's run_realgit_codex_launch scenarios
// (seam A). The originals stay in ralphloop until cutover.

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
)

// Unlike ralphloop, which leaves the ticket needs-repair, the server gives a
// failed launch's claim back: the ticket is open again and the failure streams.
func TestLaunchPort_FailureAfterClaimGivesTheClaimBack(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.PollInterval = time.Hour // one launch attempt, no retry inside the test
	})
	var prompts atomic.Int32
	h.RegisterLaunch(func(servertest.Prompt) { prompts.Add(1) })
	var starts atomic.Int32
	h.Herdr.Register("agent", "start", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		starts.Add(1)
		return nil, herdrfake.Identities{}, errors.New("Herdr rejected Codex integration")
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
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "codex"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	servertest.WaitForEvent(ctx, t, evs, server.EventIterationLaunchFailed, "proj:epic-a/01")

	if got := starts.Load(); got != 1 {
		t.Errorf("agent start calls = %d, want exactly 1", got)
	}
	if got := prompts.Load(); got != 0 {
		t.Errorf("prompts = %d, want none after a failed launch", got)
	}
	matches, _ := filepath.Glob(filepath.Join(store, "proj", "epic-a", "issues", "01-*.md"))
	if len(matches) != 1 {
		t.Fatalf("ticket files = %v", matches)
	}
	body := readFile(t, matches[0])
	if !strings.Contains(body, "status: open") {
		t.Errorf("ticket after launch failure:\n%s\nwant status: open", body)
	}
	for _, unwanted := range []string{"status: done", "status: needs-answer", "status: claimed"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("ticket after launch failure must not contain %q:\n%s", unwanted, body)
		}
	}
}

// A server that restarts mid-iteration reattaches to the live agent instead of
// prompting a fresh one, and the iteration lands exactly once.
func TestLaunchPort_RestartReattachesAndLandsOnce(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Orchestrator = config.OrchestratorServer })
	registerLiveAgent(h)
	var prompts, starts atomic.Int32
	h.Herdr.Register("agent", "prompt", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		prompts.Add(1)
		return map[string]any{}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "start", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		starts.Add(1)
		return map[string]any{}, herdrfake.Identities{}, nil
	})
	leaveIteration(t, h, store, repo)

	h.Restart(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")

	if prompts.Load() != 0 || starts.Load() != 0 {
		t.Errorf("agent starts = %d, prompts = %d, want the live session reused", starts.Load(), prompts.Load())
	}
	git := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	if got := git("show", "epic-a:agent.txt"); got != "work" {
		t.Errorf("feature branch agent.txt = %q, want the iteration's commit landed", got)
	}
	if n := strings.Count(git("log", "--format=%s", "epic-a"), "agent work"); n != 1 {
		t.Errorf("feature branch has %d agent commits, want exactly 1", n)
	}
	if body, err := os.ReadFile(filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")); err != nil || !strings.Contains(string(body), "status: done") {
		t.Errorf("ticket not done: %v\n%s", err, body)
	}
}
