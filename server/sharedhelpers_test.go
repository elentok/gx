package server_test

import (
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
)

// Aliases for the session helpers that live in servertest, so the many
// root-package callers keep their short names.
const (
	chatPollTestInterval = servertest.ChatPollTestInterval
	chatLine             = servertest.ChatLine
	codexMessageLine     = servertest.CodexMessageLine
	apiError401          = rigs.APIError401
	reply                = rigs.Reply
	turnDurationLine     = servertest.TurnDurationLine
	runEndText           = servertest.RunEndText
	repairToken          = servertest.RepairToken
	holderLabel          = servertest.HolderLabel
	investigateAddr      = servertest.InvestigateAddr
	answerAddr           = servertest.AnswerAddr
	optionsBlock         = servertest.OptionsBlock
	answerLabel          = rigs.AnswerLabel
)

type (
	headlessFake = servertest.HeadlessFake
	renamedFake  = servertest.RenamedFake
	slackHook    = rigs.SlackHook
	herdrNamed   = servertest.HerdrNamed
	codexNamed   = servertest.CodexNamed
	ptyNamed     = servertest.PtyNamed
	answerEnv    = rigs.AnswerEnv
)

var epicA01 = servertest.EpicA01

var (
	pauseBlocked = servertest.PauseBlocked
	waitStatus   = servertest.WaitStatus
)

var (
	pausedIteration         = servertest.PausedIteration
	leaveIteration          = servertest.LeaveIteration
	asHeadlessRun           = servertest.AsHeadlessRun
	stampRun                = servertest.StampRun
	runGitIn                = servertest.RunGitIn
	startWaitingRestartFake = servertest.StartWaitingRestartFake
	markWaiting             = servertest.MarkWaiting
	pauseWithQuestion       = servertest.PauseWithQuestion
	answerToken             = servertest.AnswerToken
)

const (
	epicMergedText    = rigs.EpicMergedText
	epicNotMergedText = rigs.EpicNotMergedText
)

type zeroCommitPaused = rigs.ZeroCommitPaused

var (
	writeCompleteEpic = servertest.WriteCompleteEpic
	commitOnBranch    = servertest.CommitOnBranch
	waitUntil         = servertest.WaitUntil
	atMergeCap        = servertest.AtMergeCap
	reportIteration   = servertest.ReportIteration
	keepWorktree      = servertest.KeepWorktree
	commitAgentWork   = servertest.CommitAgentWork
	runnerShapes      = servertest.RunnerShapes
	pauseZeroCommit   = rigs.PauseZeroCommit
	pausedAt          = rigs.PausedAt
	chatRecorder      = rigs.ChatRecorder
	waitForChat       = rigs.WaitForChat
	assertPaused      = rigs.AssertPaused
	legacyClass       = rigs.LegacyClass
)

var (
	newRenamedFake         = servertest.NewRenamedFake
	startInvestigateEnv    = rigs.StartInvestigateEnv
	gitIn                  = rigs.GitIn
	retryAs                = servertest.RetryAs
	epicTickets            = rigs.EpicTickets
	report                 = servertest.Report
	readMeta               = servertest.ReadMeta
	waitForHolder          = servertest.WaitForHolder
	waitForSession         = servertest.WaitForSession
	submitted              = servertest.Submitted
	waitForSubmitted       = servertest.WaitForSubmitted
	startFakeSession       = servertest.StartFakeSession
	startFakeSessionWith   = servertest.StartFakeSessionWith
	stopFakeSession        = servertest.StopFakeSession
	stoppedCodexSession    = rigs.StoppedCodexSession
	writeCodexRollout      = rigs.WriteCodexRollout
	writeTranscript        = servertest.WriteTranscript
	appendLines            = servertest.AppendLines
	appendTranscript       = servertest.AppendTranscript
	waitSessionState       = servertest.WaitSessionState
	chatEventWithin        = servertest.ChatEventWithin
	codexEventWithin       = rigs.CodexEventWithin
	failureNow             = rigs.FailureNow
	waitForFailure         = rigs.WaitForFailure
	setMapStep             = rigs.SetMapStep
	gitOut                 = servertest.GitOut
	waitFor                = servertest.Await
	reload                 = servertest.Reload
	newSlackHook           = rigs.NewSlackHook
	idleWithAction         = servertest.IdleWithAction
	withAlerts             = servertest.WithAlerts
	startBannerSession     = servertest.StartBannerSession
	reach                  = servertest.Reach
	waitAlerts             = servertest.WaitAlerts
	openTUI                = servertest.OpenTUI
	reportFocus            = servertest.ReportFocus
	screenHas              = servertest.ScreenHas
	waitForScreen          = servertest.WaitForScreen
	startGrillingSessionIn = servertest.StartGrillingSessionIn
	storeWithMapStep       = servertest.StoreWithMapStep
	writeAssistantMessage  = servertest.WriteAssistantMessage
	clearByInput           = servertest.ClearByInput
	startWebDesign         = rigs.StartWebDesign
	sessionToken           = rigs.SessionToken
	webSessionWrite        = rigs.WebSessionWrite
	startTicketWrites      = servertest.StartTicketWrites
	startBlockable         = servertest.StartBlockable
	runsJSONHas            = servertest.RunsJSONHas
	claimTicket            = servertest.ClaimTicket
	gateDwell              = servertest.GateDwell
	startWithLeftover      = servertest.StartWithLeftover
	endedRounds            = servertest.EndedRounds
	writeMuted             = servertest.WriteMuted
	runLog                 = servertest.RunLog
	ofType                 = servertest.OfType
	startDeadlock          = rigs.StartDeadlock
	runLogOf               = rigs.RunLogOf
	startAnswerEnv         = rigs.StartAnswerEnv
	pauseEnvZeroCommit     = rigs.PauseEnvZeroCommit
	answerEvents           = rigs.AnswerEvents
	stateOf                = rigs.StateOf
	mustFind               = rigs.MustFind
	registerLiveAgent      = rigs.RegisterLiveAgent
	restartUnder           = rigs.RestartUnder
	liveUnder              = rigs.LiveUnder
	liveUnderOther         = rigs.LiveUnderOther
)
