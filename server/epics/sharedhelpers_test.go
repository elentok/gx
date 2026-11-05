package epics_test

import (
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
)

// Short names for the helpers that live in servertest because the root
// package's tests use them too.
const (
	epicMergedText    = rigs.EpicMergedText
	epicNotMergedText = rigs.EpicNotMergedText
	answerAddr        = servertest.AnswerAddr
	optionsBlock      = servertest.OptionsBlock
	runEndText        = servertest.RunEndText
	repairToken       = servertest.RepairToken
)

type (
	schedFixture    = servertest.SchedFixture
	designsFixture  = servertest.DesignsFixture
	grillingFixture = servertest.GrillingFixture
)

var (
	newSchedFixture         = servertest.NewSchedFixture
	startDesigns            = servertest.StartDesigns
	startDesignsIn          = servertest.StartDesignsIn
	startAdvancing          = servertest.StartAdvancing
	endTurn                 = servertest.EndTurn
	startBlockedClaimed     = rigs.StartBlockedClaimed
	startWaitingRestartFake = servertest.StartWaitingRestartFake
	pauseWithQuestion       = servertest.PauseWithQuestion
	storeWithMapStep        = servertest.StoreWithMapStep
	commitWork              = servertest.CommitWork
	commitOnBranch          = servertest.CommitOnBranch
	writeCompleteEpic       = servertest.WriteCompleteEpic
	atMergeCap              = servertest.AtMergeCap
	runGitIn                = servertest.RunGitIn
	chatRecorder            = rigs.ChatRecorder
	waitForChat             = rigs.WaitForChat
	waitFor                 = servertest.Await
	waitUntil               = servertest.WaitUntil
	waitForSession          = servertest.WaitForSession
	report                  = servertest.Report
	readMeta                = servertest.ReadMeta
	writeAssistantMessage   = servertest.WriteAssistantMessage
)
