package sessions_test

import "github.com/elentok/gx/server/servertest"

// Aliases for the servertest helpers the moved root session tests call.
const (
	chatPollTestInterval = servertest.ChatPollTestInterval
	chatLine             = servertest.ChatLine
	codexMessageLine     = servertest.CodexMessageLine
	answerAddr           = servertest.AnswerAddr
	answerLabel          = servertest.AnswerLabel
)

var (
	waitStatus         = servertest.WaitStatus
	chatEventWithin    = servertest.ChatEventWithin
	codexEventWithin   = servertest.CodexEventWithin
	startBlockable     = servertest.StartBlockable
	startAnswerEnv     = servertest.StartAnswerEnv
	pauseEnvZeroCommit = servertest.PauseEnvZeroCommit
	runsJSONHas        = servertest.RunsJSONHas
	pauseBlocked       = servertest.PauseBlocked
)
