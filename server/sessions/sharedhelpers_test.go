package sessions_test

import (
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
)

// Aliases for the servertest helpers the moved root session tests call.
const (
	chatPollTestInterval = servertest.ChatPollTestInterval
	chatLine             = servertest.ChatLine
	codexMessageLine     = servertest.CodexMessageLine
	answerAddr           = servertest.AnswerAddr
	answerLabel          = rigs.AnswerLabel
)

var (
	waitStatus         = servertest.WaitStatus
	chatEventWithin    = servertest.ChatEventWithin
	codexEventWithin   = rigs.CodexEventWithin
	startBlockable     = servertest.StartBlockable
	startAnswerEnv     = rigs.StartAnswerEnv
	pauseEnvZeroCommit = rigs.PauseEnvZeroCommit
	runsJSONHas        = servertest.RunsJSONHas
	pauseBlocked       = servertest.PauseBlocked
)
