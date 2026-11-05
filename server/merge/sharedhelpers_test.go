package merge_test

import (
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// Short names for the helpers that live in servertest because the root
// package's tests use them too.
const (
	schedEpic         = servertest.SchedEpic
	testFlushInterval = servertest.FlushInterval
)

type (
	turnRunner       = servertest.TurnRunner
	zeroCommitPaused = servertest.ZeroCommitPaused
)

const (
	epicMergedText    = servertest.EpicMergedText
	epicNotMergedText = servertest.EpicNotMergedText
)

var (
	commitWork     = servertest.CommitWork
	newTurnRunner  = servertest.NewTurnRunner
	epicA01        = servertest.EpicA01
	gitOut         = servertest.GitOut
	runGitIn       = servertest.RunGitIn
	waitFor        = servertest.Await
	waitForSession = servertest.WaitForSession
	leaveIteration = servertest.LeaveIteration

	commitOnBranch    = servertest.CommitOnBranch
	writeCompleteEpic = servertest.WriteCompleteEpic
	waitUntil         = servertest.WaitUntil
	waitStatus        = servertest.WaitStatus
	keepWorktree      = servertest.KeepWorktree
	atMergeCap        = servertest.AtMergeCap
	runnerShapes      = servertest.RunnerShapes
	pauseZeroCommit   = servertest.PauseZeroCommit
	commitAgentWork   = servertest.CommitAgentWork
	pausedAt          = servertest.PausedAt
	assertPaused      = servertest.AssertPaused
	legacyClass       = servertest.LegacyClass
	chatRecorder      = servertest.ChatRecorder
	waitForChat       = servertest.WaitForChat
	reportIteration   = servertest.ReportIteration
)

// schedFixture adds the merge-only helpers (other files' methods) to the shared
// servertest.SchedFixture. Its callbacks receive the wrapper.
type schedFixture struct {
	*servertest.SchedFixture
}

func retroFixture(t *testing.T, opts ...func(*server.Config)) *schedFixture {
	t.Helper()
	return &schedFixture{servertest.RetroFixture(t, opts...)}
}

func newSchedFixture(t *testing.T, tickets map[string]servertest.TicketOpts, agentTurn func(f *schedFixture, p *servertest.Turn, id string), cfgOpts ...func(*server.Config)) *schedFixture {
	t.Helper()
	f := &schedFixture{}
	var inner func(*servertest.SchedFixture, *servertest.Turn, string)
	if agentTurn != nil {
		inner = func(_ *servertest.SchedFixture, p *servertest.Turn, id string) { agentTurn(f, p, id) }
	}
	f.SchedFixture = servertest.NewSchedFixture(t, tickets, inner, cfgOpts...)
	return f
}
