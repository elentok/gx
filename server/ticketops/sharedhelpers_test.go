package ticketops_test

import (
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
)

// Aliases for the shared helpers that live in servertest, so the moved tests
// keep their short names.
const (
	investigateAddr = servertest.InvestigateAddr
	answerAddr      = servertest.AnswerAddr
	optionsBlock    = servertest.OptionsBlock
	answerLabel     = rigs.AnswerLabel
)

type (
	headlessFake = servertest.HeadlessFake
	renamedFake  = servertest.RenamedFake
	herdrNamed   = servertest.HerdrNamed
	ptyNamed     = servertest.PtyNamed
	turnRunner   = servertest.TurnRunner
	answerEnv    = rigs.AnswerEnv
)

var epicA01 = servertest.EpicA01

var (
	startTicketWrites       = servertest.StartTicketWrites
	endedRounds             = servertest.EndedRounds
	keepWorktree            = servertest.KeepWorktree
	writeMuted              = servertest.WriteMuted
	assertPaused            = rigs.AssertPaused
	commitAgentWork         = servertest.CommitAgentWork
	newTurnRunner           = servertest.NewTurnRunner
	pauseZeroCommit         = rigs.PauseZeroCommit
	reportIteration         = servertest.ReportIteration
	runLogOf                = rigs.RunLogOf
	startDeadlock           = rigs.StartDeadlock
	startInvestigateEnv     = rigs.StartInvestigateEnv
	waitStatus              = servertest.WaitStatus
	startWaitingRestartFake = servertest.StartWaitingRestartFake
	pauseWithQuestion       = servertest.PauseWithQuestion
	newRenamedFake          = servertest.NewRenamedFake
	waitFor                 = servertest.Await
	startAnswerEnv          = rigs.StartAnswerEnv
	mustFind                = rigs.MustFind
	answerEvents            = rigs.AnswerEvents
	liveUnder               = rigs.LiveUnder
	liveUnderOther          = rigs.LiveUnderOther
	pausedIteration         = servertest.PausedIteration
	stateOf                 = rigs.StateOf
	pauseEnvZeroCommit      = rigs.PauseEnvZeroCommit
	newSchedFixture         = servertest.NewSchedFixture
	commitWork              = servertest.CommitWork
	atMergeCap              = servertest.AtMergeCap
)

const schedEpic = servertest.SchedEpic
