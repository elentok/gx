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
)

var (
	report               = servertest.Report
	readMeta             = servertest.ReadMeta
	waitForHolder        = servertest.WaitForHolder
	waitForSession       = servertest.WaitForSession
	submitted            = servertest.Submitted
	waitForSubmitted     = servertest.WaitForSubmitted
	startFakeSession     = servertest.StartFakeSession
	startFakeSessionWith = servertest.StartFakeSessionWith
	stopFakeSession      = servertest.StopFakeSession
	stoppedCodexSession  = servertest.StoppedCodexSession
	writeCodexRollout    = servertest.WriteCodexRollout
	writeTranscript      = servertest.WriteTranscript
	appendLines          = servertest.AppendLines
	appendTranscript     = servertest.AppendTranscript
	waitSessionState     = servertest.WaitSessionState
	chatEventWithin      = servertest.ChatEventWithin
	codexEventWithin     = servertest.CodexEventWithin
	failureNow           = servertest.FailureNow
	waitForFailure       = servertest.WaitForFailure
	setMapStep           = servertest.SetMapStep
)
