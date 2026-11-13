package mergeflows_test

import (
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/merge/mergetest"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
)

const schedEpic = servertest.SchedEpic

var (
	commitWork        = servertest.CommitWork
	gitOut            = servertest.GitOut
	runGitIn          = servertest.RunGitIn
	waitFor           = servertest.Await
	waitForSession    = servertest.WaitForSession
	commitOnBranch    = servertest.CommitOnBranch
	writeCompleteEpic = servertest.WriteCompleteEpic
	waitUntil         = servertest.WaitUntil
	waitStatus        = servertest.WaitStatus
	keepWorktree      = servertest.KeepWorktree
	atMergeCap        = servertest.AtMergeCap
	assertPaused      = rigs.AssertPaused
	legacyClass       = rigs.LegacyClass
	reportIteration   = servertest.ReportIteration
)

// schedFixture adds the merge-only helpers (other files' methods) to the shared
// mergetest.Fixture. Its callbacks receive the wrapper.
type schedFixture struct {
	*mergetest.Fixture
}

func retroFixture(t *testing.T, opts ...func(*server.Config)) *schedFixture {
	t.Helper()
	return &schedFixture{&mergetest.Fixture{SchedFixture: servertest.RetroFixture(t, opts...)}}
}

func newSchedFixture(t *testing.T, tickets map[string]servertest.TicketOpts, agentTurn func(f *schedFixture, p *servertest.Turn, id string), cfgOpts ...func(*server.Config)) *schedFixture {
	t.Helper()
	f := &schedFixture{Fixture: &mergetest.Fixture{}}
	var inner func(*servertest.SchedFixture, *servertest.Turn, string)
	if agentTurn != nil {
		inner = func(_ *servertest.SchedFixture, p *servertest.Turn, id string) { agentTurn(f, p, id) }
	}
	f.SchedFixture = servertest.NewSchedFixture(t, tickets, inner, cfgOpts...)
	return f
}
