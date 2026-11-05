package recovery_test

import "github.com/elentok/gx/server/servertest"

// Short names for the helpers that live in servertest because the root
// package's tests use them too.
const (
	holderLabel       = servertest.HolderLabel
	testFlushInterval = servertest.FlushInterval
	answerAddr        = servertest.AnswerAddr
	optionsBlock      = servertest.OptionsBlock
)

type renamedFake = servertest.RenamedFake

var (
	waitFor             = servertest.Await
	gitOut              = servertest.GitOut
	claimTicket         = servertest.ClaimTicket
	ofType              = servertest.OfType
	runLog              = servertest.RunLog
	newRenamedFake      = servertest.NewRenamedFake
	startInvestigateEnv = servertest.StartInvestigateEnv
	gitIn               = servertest.GitIn
	epicTickets         = servertest.EpicTickets

	pausedIteration         = servertest.PausedIteration
	startWaitingRestartFake = servertest.StartWaitingRestartFake
	pauseWithQuestion       = servertest.PauseWithQuestion
)
