package sched_test

import (
	"testing"

	"github.com/elentok/gx/server/servertest"
)

// Short names for the helpers that live in servertest because the root
// package's tests use them too.
const (
	answerAddr        = servertest.AnswerAddr
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
	assertPaused            = servertest.AssertPaused
	chatRecorder            = servertest.ChatRecorder
	commitAgentWork         = servertest.CommitAgentWork
	newSlackHook            = servertest.NewSlackHook
	pauseBlocked            = servertest.PauseBlocked
	startBlockable          = servertest.StartBlockable
	startTicketWrites       = servertest.StartTicketWrites
	waitForChat             = servertest.WaitForChat
	leaveIteration          = servertest.LeaveIteration
	markWaiting             = servertest.MarkWaiting
	newRenamedFake          = servertest.NewRenamedFake
	ofType                  = servertest.OfType
	registerLiveAgent       = servertest.RegisterLiveAgent
	restartUnder            = servertest.RestartUnder
	runLog                  = servertest.RunLog
	runLogOf                = servertest.RunLogOf
	epicTickets             = servertest.EpicTickets
	startDeadlock           = servertest.StartDeadlock
	runnerShapes            = servertest.RunnerShapes
	runsJSONHas             = servertest.RunsJSONHas
	stampRun                = servertest.StampRun
	startWaitingRestartFake = servertest.StartWaitingRestartFake
	waitFor                 = servertest.Await
	liveUnder               = servertest.LiveUnder
	liveUnderOther          = servertest.LiveUnderOther
	pauseWithQuestion       = servertest.PauseWithQuestion
)

const optionsBlock = servertest.OptionsBlock

// leaveIterationWith is leaveIteration with work as what the agent did in its worktree.
func leaveIterationWith(t *testing.T, h *servertest.Harness, store, repo, runner string, work func(wtPath string)) (string, string) {
	t.Helper()
	return h.LeaveHerdrRun(t, store, repo, epicA01, runner, work)
}
