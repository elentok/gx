package launchtest_test

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/herdrrunner"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/sessionlookup"
	"github.com/elentok/gx/tickets/schema"
)

// findErrFake fails every label lookup, which runnerfake cannot inject.
type findErrFake struct {
	renamedFake
	err error
}

func (r findErrFake) Find(string) (agentrunner.Session, bool, error) {
	return agentrunner.Session{}, false, r.err
}

var waitStatus = servertest.WaitStatus

func TestReclaim_FinishesThroughTheRunsOwnRunner(t *testing.T) {
	t.Parallel()
	own, configured := newRenamedFake("a", nativerunner.VerdictFinished), newRenamedFake("b", nativerunner.VerdictFinished)
	_, ticketPath := restartUnder(t, "a", configured, map[string]agentrunner.Runner{"a": own})
	waitStatus(t, ticketPath, schema.StatusDone)
	servertest.Poll(10*time.Second, func() bool { return len(own.Cleaned()) > 0 })
	if got := own.Cleaned(); !slices.Equal(got, []string{"epic-a-iter-01"}) {
		t.Errorf("run's runner cleaned %v, want epic-a-iter-01", got)
	}
	if got := configured.Cleaned(); len(got) != 0 {
		t.Errorf("configured runner cleaned %v, want nothing", got)
	}
}

// Regression: a herdr run reclaimed under a headless server was waited on and
// finished through headless, which had no such session, and was left unwatched.
func TestReclaim_HerdrRunUnderAHeadlessServerFinishesThroughHerdr(t *testing.T) {
	t.Parallel()
	headless := newRenamedFake("headless", nativerunner.VerdictLive)
	others := map[string]agentrunner.Runner{}
	// The runner reaches the fake herdr through the server's own env.
	_, ticketPath := restartUnder(t, "", headless, others, servertest.WithHerdr(servertest.HerdrIsTheSubject), func(c *server.Config) {
		others["herdr"] = herdrrunner.NewIn(herdr.CLI{Env: c.Env}, harness.LookupEnv(c.Env, "HOME"), sessionlookup.RootsFrom(c.Env).CodexHome)
	})
	waitStatus(t, ticketPath, schema.StatusDone)
}

func TestReclaim_NonMismatchErrorPausesStuckWithItsReason(t *testing.T) {
	t.Parallel()
	// The handle names herdr, so reclaim looks the agent up by label on this runner.
	lookup := findErrFake{newRenamedFake("herdr", nativerunner.VerdictLive), errors.New("lookup refused")}
	_, ticketPath := restartUnder(t, "", lookup, nil)
	waitStatus(t, ticketPath, schema.StatusLegacyRepair)
	body, _ := os.ReadFile(ticketPath)
	if !strings.Contains(string(body), "lookup refused") {
		t.Errorf("ticket lacks the reclaim error in its reason:\n%s", body)
	}
}

func TestReclaim_UnavailableRunnerPausesStuckNamingIt(t *testing.T) {
	t.Parallel()
	_, ticketPath := restartUnder(t, "pty", newRenamedFake("headless", nativerunner.VerdictLive), nil)
	waitStatus(t, ticketPath, schema.StatusLegacyRepair)
	body, _ := os.ReadFile(ticketPath)
	if !strings.Contains(string(body), "started by pty; pty unavailable") {
		t.Errorf("ticket lacks the runner in its reason:\n%s", body)
	}
}
