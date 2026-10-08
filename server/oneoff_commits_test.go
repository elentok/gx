package server_test

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

// startCommitsOneOff submits a --commits one-off against a real repo whose
// agent commits one file, and returns the stream, address and repo. seen is
// the branch the agent's worktree was on and trunk's tip while it worked.
func startCommitsOneOff(t *testing.T, noAutoMerge bool) (evs <-chan server.Event, addr, repo string, seen *oneOffSeen) {
	t.Helper()
	store := t.TempDir()
	repo = testutil.TempRepo(t)
	testutil.Mkdir(t, filepath.Join(store, "proj"))
	if noAutoMerge {
		servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	} else {
		servertest.SetProjectRepo(t, store, "proj", repo)
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.PollInterval = 50 * time.Millisecond
	})
	seen = &oneOffSeen{}
	h.RegisterLaunch(func(p servertest.Prompt) {
		seen.record(gitOut(t, p.Cwd, "branch", "--show-current"), gitOut(t, repo, "rev-parse", "main"))
		commitWork(t, p, "work")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if evs, err = h.Client.Events(ctx, snap.Seq); err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.OneOff(ctx, server.OneOffRequest{Prompt: "add a file", Project: "proj", Name: "oneoff", Commits: true})
	if err != nil || res.Refused {
		t.Fatalf("one-off: %+v, %v", res, err)
	}
	return evs, res.Address, repo, seen
}

type oneOffSeen struct {
	mu            sync.Mutex
	branch, trunk string
}

func (s *oneOffSeen) record(branch, trunk string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.branch, s.trunk = branch, trunk
}

func (s *oneOffSeen) get() (branch, trunk string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.branch, s.trunk
}

func TestOneOffCommits_RunsOnAnIterationBranchAndLandsThroughThePolicy(t *testing.T) {
	evs, addr, repo, seen := startCommitsOneOff(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")

	branch, trunkWhileRunning := seen.get()
	if !strings.HasPrefix(branch, "ralph-loop/oneoff") {
		t.Errorf("agent ran on branch %q, want an iteration branch", branch)
	}
	if got := gitOut(t, repo, "rev-parse", "main"); got == trunkWhileRunning {
		t.Errorf("main never moved after the root completed (%s)", addr)
	}
	if got := gitOut(t, repo, "show", "main:work.txt"); got != "work" {
		t.Errorf("main:work.txt = %q", got)
	}
	if got := gitOut(t, repo, "rev-parse", "main"); got != gitOut(t, repo, "rev-parse", "oneoff") {
		t.Errorf("main %s is not the one-off's feature branch tip", got)
	}
}

func TestOneOffCommits_AutoFFOffParksTheRootAndLeavesTrunkAlone(t *testing.T) {
	evs, _, repo, seen := startCommitsOneOff(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	servertest.WaitForEvent(ctx, t, evs, server.EventRootParked, "")

	_, trunkWhileRunning := seen.get()
	if got := gitOut(t, repo, "rev-parse", "main"); got != trunkWhileRunning {
		t.Errorf("main moved to %s without the landing policy", got)
	}
}
