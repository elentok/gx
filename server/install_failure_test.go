package server_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/herdrfake"
)

// installFailure is the fake install of a worktree whose dependencies will not install.
func installFailure(path, _ string) ([]string, error) {
	return nil, errors.New("installing dependencies in " + path + ": boom")
}

// runFailingInstall queues ticket 01 of a fixture whose install fails and waits
// for the launch to be given up on. starts counts the agent start calls.
func runFailingInstall(t *testing.T, typ string) (f *schedFixture, starts *atomic.Int32) {
	t.Helper()
	f = newSchedFixture(t, map[string]servertest.TicketOpts{"01": {Type: typ}},
		func(*schedFixture, servertest.Prompt, string) {},
		func(c *server.Config) { c.InstallDeps = installFailure })
	gitOut(t, f.repo, "branch", schedEpic)
	servertest.SetProjectRepoNoAutoMerge(t, f.store, "proj", f.repo)
	starts = &atomic.Int32{}
	f.h.Herdr.Register("agent", "start", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		starts.Add(1)
		return nil, herdrfake.Identities{}, errors.New("agent must not start")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventIterationLaunchFailed, "proj:"+schedEpic+"/01")
	return f, starts
}

func assertPausedOnInstallFailure(t *testing.T, f *schedFixture, starts *atomic.Int32) {
	t.Helper()
	body := readFile(t, f.ticketFile(t, "01"))
	if !strings.Contains(body, "status: paused") {
		t.Errorf("ticket not paused:\n%s", body)
	}
	if !strings.Contains(body, "launch failed: installing dependencies in") {
		t.Errorf("pause reason missing the install failure:\n%s", body)
	}
	if got := starts.Load(); got != 0 {
		t.Errorf("agent start calls = %d, want none", got)
	}
}

func TestInstallFailure_PausesAndDiscardsTheIterationWorktree(t *testing.T) {
	f, starts := runFailingInstall(t, "implement")

	assertPausedOnInstallFailure(t, f, starts)
	if out := strings.TrimSpace(f.git(t, "branch", "--list", "ralph-loop/*")); out != "" {
		t.Errorf("iteration branch left behind: %s", out)
	}
	if out := f.git(t, "worktree", "list"); strings.Count(strings.TrimSpace(out), "\n") != 1 {
		t.Errorf("want only the repo and the feature worktree, got:\n%s", out)
	}
	f.assertFeatureKept(t, "install failure")
}

func TestInstallFailure_MergeTicketKeepsTheFeatureWorktree(t *testing.T) {
	f, starts := runFailingInstall(t, "merge")

	assertPausedOnInstallFailure(t, f, starts)
	f.assertFeatureKept(t, "install failure")
}
