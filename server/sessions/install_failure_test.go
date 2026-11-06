package sessions_test

import (
	"context"
	"errors"
	"github.com/elentok/gx/server/wire"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// installFailure is the fake install of a worktree whose dependencies will not install.
func installFailure(_ context.Context, path, _ string) ([]string, error) {
	return nil, errors.New("installing dependencies in " + path + ": boom")
}

// runFailingInstall queues ticket 01 of a fixture whose install fails and waits
// for the launch to be given up on. starts counts the agent start calls.
func runFailingInstall(t *testing.T, typ string) (f *schedFixture, starts func() []servertest.AgentStart) {
	t.Helper()
	f = newSchedFixture(t, map[string]servertest.TicketOpts{"01": {Type: typ}},
		func(*schedFixture, *servertest.Turn, string) {},
		func(c *server.Config) { c.InstallDeps = installFailure })
	gitOut(t, f.Repo, "branch", schedEpic)
	servertest.SetProjectRepoNoAutoMerge(t, f.Store, "proj", f.Repo)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventIterationLaunchFailed, "proj:"+schedEpic+"/01")
	return f, f.H.Starts
}

func assertPausedOnInstallFailure(t *testing.T, f *schedFixture, starts func() []servertest.AgentStart) {
	t.Helper()
	body := servertest.ReadFile(t, f.TicketFile(t, "01"))
	if !strings.Contains(body, "status: paused") {
		t.Errorf("ticket not paused:\n%s", body)
	}
	if !strings.Contains(body, "launch failed: installing dependencies in") {
		t.Errorf("pause reason missing the install failure:\n%s", body)
	}
	if got := starts(); len(got) != 0 {
		t.Errorf("agent starts = %v, want none", got)
	}
}

func TestInstallFailure_PausesAndDiscardsTheIterationWorktree(t *testing.T) {
	t.Parallel()
	f, starts := runFailingInstall(t, "implement")

	assertPausedOnInstallFailure(t, f, starts)
	if out := strings.TrimSpace(f.Git(t, "branch", "--list", "ralph-loop/*")); out != "" {
		t.Errorf("iteration branch left behind: %s", out)
	}
	if out := f.Git(t, "worktree", "list"); strings.Count(strings.TrimSpace(out), "\n") != 1 {
		t.Errorf("want only the repo and the feature worktree, got:\n%s", out)
	}
	f.AssertFeatureKept(t, "install failure")
}

func TestInstallFailure_MergeTicketKeepsTheFeatureWorktree(t *testing.T) {
	t.Parallel()
	f, starts := runFailingInstall(t, "merge")

	assertPausedOnInstallFailure(t, f, starts)
	f.AssertFeatureKept(t, "install failure")
}
