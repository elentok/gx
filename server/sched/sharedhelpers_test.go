package sched_test

import (
	"testing"

	"github.com/elentok/gx/server/servertest"
)

// Short names for the helpers that live in servertest because the root
// package's tests use them too.
const holderLabel = servertest.HolderLabel

type (
	headlessFake = servertest.HeadlessFake
	renamedFake  = servertest.RenamedFake
)

var epicA01 = servertest.EpicA01

var (
	asHeadlessRun           = servertest.AsHeadlessRun
	assertPaused            = servertest.AssertPaused
	commitAgentWork         = servertest.CommitAgentWork
	leaveIteration          = servertest.LeaveIteration
	markWaiting             = servertest.MarkWaiting
	newRenamedFake          = servertest.NewRenamedFake
	ofType                  = servertest.OfType
	registerLiveAgent       = servertest.RegisterLiveAgent
	restartUnder            = servertest.RestartUnder
	runLog                  = servertest.RunLog
	runnerShapes            = servertest.RunnerShapes
	runsJSONHas             = servertest.RunsJSONHas
	stampRun                = servertest.StampRun
	startWaitingRestartFake = servertest.StartWaitingRestartFake
	waitFor                 = servertest.Await
)

// leaveIterationWith is leaveIteration with work as what the agent did in its worktree.
func leaveIterationWith(t *testing.T, h *servertest.Harness, store, repo, runner string, work func(wtPath string)) (string, string) {
	t.Helper()
	return h.LeaveHerdrRun(t, store, repo, epicA01, runner, work)
}
