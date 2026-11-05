package worktree

import (
	"errors"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/testutil/runnerfake"
)

func TestFindIterationSession(t *testing.T) {
	r := runnerfake.NewRunner()
	d := deps.Deps{Runner: r}

	if _, ok := FindIterationSession(d, "epic", "01"); ok {
		t.Fatal("found a session before any was started")
	}

	want, err := r.Start(agentrunner.StartOptions{Label: IterationLabel("epic", "01"), Epic: "epic"})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := FindIterationSession(d, "epic", "01")
	if !ok || got != want {
		t.Fatalf("FindIterationSession = %+v, %v; want %+v", got, ok, want)
	}
	if _, ok := FindIterationSession(d, "epic", "02"); ok {
		t.Fatal("found another ticket's session")
	}
}

func TestFindIterationSession_NoRunnerReportsNone(t *testing.T) {
	if _, ok := FindIterationSession(deps.Deps{}, "epic", "01"); ok {
		t.Fatal("nil runner reported a session")
	}
}

func TestFindIterationSession_Unreachable(t *testing.T) {
	d := deps.Deps{Runner: findErrRunner{runnerfake.NewRunner()}}
	if _, ok := FindIterationSession(d, "epic", "01"); ok {
		t.Fatal("Find error reported a session")
	}
}

type findErrRunner struct{ *runnerfake.Runner }

func (findErrRunner) Find(string) (agentrunner.Session, bool, error) {
	return agentrunner.Session{}, false, errors.New("host down")
}
