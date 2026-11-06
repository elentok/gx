package launchtest_test

import (
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
)

// Short names for the helpers that live in servertest because the root
// package's tests use them too.
const (
	holderLabel       = servertest.HolderLabel
	testFlushInterval = servertest.FlushInterval
)

type (
	headlessFake = servertest.HeadlessFake
	renamedFake  = servertest.RenamedFake
)

var epicA01 = servertest.EpicA01

var (
	asHeadlessRun           = servertest.AsHeadlessRun
	commitAgentWork         = servertest.CommitAgentWork
	newSlackHook            = rigs.NewSlackHook
	startTicketWrites       = servertest.StartTicketWrites
	leaveIteration          = servertest.LeaveIteration
	newRenamedFake          = servertest.NewRenamedFake
	ofType                  = servertest.OfType
	registerLiveAgent       = rigs.RegisterLiveAgent
	restartUnder            = rigs.RestartUnder
	runLog                  = servertest.RunLog
	runnerShapes            = servertest.RunnerShapes
	stampRun                = servertest.StampRun
	startWaitingRestartFake = servertest.StartWaitingRestartFake
	waitFor                 = servertest.Await
)

// leaveIterationWith is leaveIteration with work as what the agent did in its worktree.
func leaveIterationWith(t *testing.T, h *servertest.Harness, store, repo, runner string, work func(wtPath string)) (string, string) {
	t.Helper()
	return h.LeaveHerdrRun(t, store, repo, epicA01, runner, work)
}
