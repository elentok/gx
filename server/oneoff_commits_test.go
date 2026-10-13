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
func startCommitsOneOff(t *testing.T, noAutoMerge bool, cfgOpts ...func(*server.Config)) (evs <-chan server.Event, addr, repo string, seen *oneOffSeen) {
	t.Helper()
	seen = &oneOffSeen{}
	evs, addr, repo = startCommitsOneOffWith(t, noAutoMerge, func(p servertest.Prompt, _, repo string) {
		seen.record(gitOut(t, p.Cwd, "branch", "--show-current"), gitOut(t, repo, "rev-parse", "main"))
		commitWork(t, p, "work")
	}, cfgOpts...)
	return evs, addr, repo, seen
}

// startCommitsOneOffWith is startCommitsOneOff whose agent runs turn.
func startCommitsOneOffWith(t *testing.T, noAutoMerge bool, turn func(p servertest.Prompt, store, repo string), cfgOpts ...func(*server.Config)) (evs <-chan server.Event, addr, repo string) {
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
		for _, o := range cfgOpts {
			o(c)
		}
	})
	h.RegisterLaunch(func(p servertest.Prompt) { turn(p, store, repo) })
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
	return evs, res.Address, repo
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

func TestOneOffCommits_AutoFFOffCompletesTheRootAndLeavesTrunkAlone(t *testing.T) {
	evs, _, repo, seen := startCommitsOneOff(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")

	_, trunkWhileRunning := seen.get()
	if got := gitOut(t, repo, "rev-parse", "main"); got != trunkWhileRunning {
		t.Errorf("main moved to %s without the landing policy", got)
	}
}
