package web_test

import "github.com/elentok/gx/server/servertest"

// Short names for the helpers that live in servertest.
const (
	optionsBlock = servertest.OptionsBlock
	answerAddr   = servertest.AnswerAddr
)

var (
	startWaitingRestartFake = servertest.StartWaitingRestartFake
	pauseWithQuestion       = servertest.PauseWithQuestion
	waitFor                 = servertest.Await
	reload                  = servertest.Reload
	startTicketWrites       = servertest.StartTicketWrites
	endedRounds             = servertest.EndedRounds
	writeMuted              = servertest.WriteMuted
)
