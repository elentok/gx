package server_test

import "github.com/elentok/gx/server/servertest"

// Aliases for the session helpers that live in servertest, so the many
// root-package callers keep their short names.
const (
	chatPollTestInterval = servertest.ChatPollTestInterval
	chatLine             = servertest.ChatLine
	codexMessageLine     = servertest.CodexMessageLine
	apiError401          = servertest.APIError401
	reply                = servertest.Reply
	turnDurationLine     = servertest.TurnDurationLine
	runEndText           = servertest.RunEndText
	repairToken          = servertest.RepairToken
	holderLabel          = servertest.HolderLabel
	investigateAddr      = servertest.InvestigateAddr
)

type (
	headlessFake = servertest.HeadlessFake
	renamedFake  = servertest.RenamedFake
	slackHook    = servertest.SlackHook
	herdrNamed   = servertest.HerdrNamed
	codexNamed   = servertest.CodexNamed
	ptyNamed     = servertest.PtyNamed
)

var (
	newRenamedFake         = servertest.NewRenamedFake
	startInvestigateEnv    = servertest.StartInvestigateEnv
	gitIn                  = servertest.GitIn
	retryAs                = servertest.RetryAs
	epicTickets            = servertest.EpicTickets
	report                 = servertest.Report
	readMeta               = servertest.ReadMeta
	waitForHolder          = servertest.WaitForHolder
	waitForSession         = servertest.WaitForSession
	submitted              = servertest.Submitted
	waitForSubmitted       = servertest.WaitForSubmitted
	startFakeSession       = servertest.StartFakeSession
	startFakeSessionWith   = servertest.StartFakeSessionWith
	stopFakeSession        = servertest.StopFakeSession
	stoppedCodexSession    = servertest.StoppedCodexSession
	writeCodexRollout      = servertest.WriteCodexRollout
	writeTranscript        = servertest.WriteTranscript
	appendLines            = servertest.AppendLines
	appendTranscript       = servertest.AppendTranscript
	waitSessionState       = servertest.WaitSessionState
	chatEventWithin        = servertest.ChatEventWithin
	codexEventWithin       = servertest.CodexEventWithin
	failureNow             = servertest.FailureNow
	waitForFailure         = servertest.WaitForFailure
	setMapStep             = servertest.SetMapStep
	gitOut                 = servertest.GitOut
	waitFor                = servertest.Await
	reload                 = servertest.Reload
	newSlackHook           = servertest.NewSlackHook
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
	startWebDesign         = servertest.StartWebDesign
	sessionToken           = servertest.SessionToken
	webSessionWrite        = servertest.WebSessionWrite
	startTicketWrites      = servertest.StartTicketWrites
	startBlockable         = servertest.StartBlockable
	runsJSONHas            = servertest.RunsJSONHas
	claimTicket            = servertest.ClaimTicket
	gateDwell              = servertest.GateDwell
	startWithLeftover      = servertest.StartWithLeftover
	endedRounds            = servertest.EndedRounds
	runLog                 = servertest.RunLog
	ofType                 = servertest.OfType
)
