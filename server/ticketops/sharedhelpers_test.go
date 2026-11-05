package ticketops_test

import "github.com/elentok/gx/server/servertest"

// Aliases for the shared helpers that live in servertest, so the moved tests
// keep their short names.
const (
	investigateAddr = servertest.InvestigateAddr
	answerAddr      = servertest.AnswerAddr
	optionsBlock    = servertest.OptionsBlock
	answerLabel     = servertest.AnswerLabel
)

type (
	headlessFake = servertest.HeadlessFake
	renamedFake  = servertest.RenamedFake
	herdrNamed   = servertest.HerdrNamed
	ptyNamed     = servertest.PtyNamed
	turnRunner   = servertest.TurnRunner
	answerEnv    = servertest.AnswerEnv
)

var (
	assertPaused            = servertest.AssertPaused
	commitAgentWork         = servertest.CommitAgentWork
	newTurnRunner           = servertest.NewTurnRunner
	pauseZeroCommit         = servertest.PauseZeroCommit
	reportIteration         = servertest.ReportIteration
	runLogOf                = servertest.RunLogOf
	startDeadlock           = servertest.StartDeadlock
	startInvestigateEnv     = servertest.StartInvestigateEnv
	waitStatus              = servertest.WaitStatus
	startWaitingRestartFake = servertest.StartWaitingRestartFake
	pauseWithQuestion       = servertest.PauseWithQuestion
	newRenamedFake          = servertest.NewRenamedFake
	waitFor                 = servertest.Await
	startAnswerEnv          = servertest.StartAnswerEnv
	mustFind                = servertest.MustFind
	answerEvents            = servertest.AnswerEvents
	liveUnder               = servertest.LiveUnder
	liveUnderOther          = servertest.LiveUnderOther
)
