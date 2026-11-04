package server_test

import (
	"github.com/elentok/gx/server/wire"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server/servertest"
)

// trunkDeletedMidRun is a one-ticket epic whose trunk develop is deleted while
// the agent works. It returns develop's tip and a count of chat text sent.
func trunkDeletedMidRun(t *testing.T, autoMerge bool) (f *schedFixture, develop string, chatCount func(string) int) {
	t.Helper()
	chatCfg, chatCount := chatRecorder(t)
	f = newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
		f.Git(t, "branch", "-D", "develop")
	}, chatCfg)
	f.Git(t, "branch", "develop")
	if autoMerge {
		servertest.SetProjectTrunk(t, f.Store, "proj", f.Repo, "develop")
	} else {
		servertest.SetProjectTrunkNoAutoMerge(t, f.Store, "proj", f.Repo, "develop")
	}
	return f, f.rev(t, "develop"), chatCount
}

func TestTrunkMissingAtMerging_TicketMergesOnItsRootAndTheRootPauses(t *testing.T) {
	t.Parallel()
	f, develop, chatCount := trunkDeletedMidRun(t, true)
	f.runEpic(t, wire.EventRootPaused)

	if body := servertest.ReadFile(t, f.TicketFile(t, "01")); !strings.Contains(body, "status: done") {
		t.Errorf("ticket did not merge on its root's branch:\n%s", body)
	}
	if got := f.Git(t, "show", schedEpic+":01.txt"); got != "01" {
		t.Errorf("%s:01.txt = %q, want the ticket's work", schedEpic, got)
	}
	if got := f.rev(t, "main"); got != develop {
		t.Errorf("main moved to %s", got)
	}
	if waitForChat(chatCount, "the trunk branch is missing: project") == 0 {
		t.Errorf("no root pause with reason trunk-missing reached chat")
	}
	if slices.Contains(f.H.Server.LockedTrunks(), "") {
		t.Errorf("a merge locked on an empty target")
	}
}

func TestTrunkMissingAtMerging_AutoMergeOffIsNotPaused(t *testing.T) {
	t.Parallel()
	f, _, _ := trunkDeletedMidRun(t, false)
	f.runEpic(t, wire.EventRootCompleted)

	if body := servertest.ReadFile(t, f.TicketFile(t, "01")); !strings.Contains(body, "status: done") {
		t.Errorf("ticket not done:\n%s", body)
	}
	if slices.Contains(f.H.Server.LockedTrunks(), "") {
		t.Errorf("a merge locked on an empty target")
	}
}
