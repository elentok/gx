package launchtest_test

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/tickets/schema"
)

// reattachErrFake fails every reattach lookup, which runnerfake cannot inject.
type reattachErrFake struct {
	renamedFake
	err error
}

func (r reattachErrFake) Reattach(string) (agentrunner.Session, nativerunner.Verdict, error) {
	return agentrunner.Session{}, nativerunner.VerdictLive, r.err
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

func TestReclaim_NonMismatchErrorPausesStuckWithItsReason(t *testing.T) {
	t.Parallel()
	lookup := reattachErrFake{newRenamedFake("a", nativerunner.VerdictLive), errors.New("lookup refused")}
	_, ticketPath := restartUnder(t, "a", lookup, nil)
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
	if !strings.Contains(string(body), "started by pty; the pty runner was removed. Retry or Start over ends this pause.") {
		t.Errorf("ticket lacks the runner in its reason:\n%s", body)
	}
}
