package sched_test

import (
	"github.com/elentok/gx/server/servertest/rigs"
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
	assertPaused            = rigs.AssertPaused
	chatRecorder            = rigs.ChatRecorder
	commitAgentWork         = servertest.CommitAgentWork
	newSlackHook            = rigs.NewSlackHook
	pauseBlocked            = servertest.PauseBlocked
	startBlockable          = servertest.StartBlockable
	startTicketWrites       = servertest.StartTicketWrites
	waitForChat             = rigs.WaitForChat
	leaveIteration          = servertest.LeaveIteration
	markWaiting             = servertest.MarkWaiting
	newRenamedFake          = servertest.NewRenamedFake
	ofType                  = servertest.OfType
	registerLiveAgent       = rigs.RegisterLiveAgent
	restartUnder            = rigs.RestartUnder
	runLog                  = servertest.RunLog
	runLogOf                = rigs.RunLogOf
	epicTickets             = rigs.EpicTickets
	startDeadlock           = rigs.StartDeadlock
	runnerShapes            = servertest.RunnerShapes
	runsJSONHas             = servertest.RunsJSONHas
	stampRun                = servertest.StampRun
	startWaitingRestartFake = servertest.StartWaitingRestartFake
	waitFor                 = servertest.Await
	liveUnder               = rigs.LiveUnder
	liveUnderOther          = rigs.LiveUnderOther
	pauseWithQuestion       = servertest.PauseWithQuestion
)

const optionsBlock = servertest.OptionsBlock

// leaveIterationWith is leaveIteration with work as what the agent did in its worktree.
func leaveIterationWith(t *testing.T, h *servertest.Harness, store, repo, runner string, work func(wtPath string)) (string, string) {
	t.Helper()
	return h.LeaveHerdrRun(t, store, repo, epicA01, runner, work)
}
