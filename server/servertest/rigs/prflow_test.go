package rigs_test

import (
	"os"
	"testing"

	"github.com/elentok/gx/git/pr"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/testutil/ghfake"
	"github.com/elentok/gx/testutil/herdrfake"
)

// The servertest harness runs this test binary as its session holder and as
// the fake herdr; without both calls the binary re-runs the suite in every
// child and forks without end.
func TestMain(m *testing.M) {
	servertest.RunSessionProcesses()
	herdrfake.RunHelperProcess()
	ghfake.Warm()
	code := m.Run()
	servertest.RemoveFakeCodex()
	os.Exit(code)
}

func TestStartPRFlow_ServerEnvRunsTheFakeGH(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	repo, err := pr.Runner{Env: f.Env, Dir: f.Origin.Local}.Preflight()
	if err != nil {
		t.Fatal(err)
	}
	if repo != ghfake.Repo {
		t.Fatalf("repo = %q, want %q", repo, ghfake.Repo)
	}
	if calls := f.GH.CallsMatching("repo view " + ghfake.Repo); len(calls) != 1 {
		t.Fatalf("fake gh saw %v, want one repo view", f.GH.Calls())
	}
}
