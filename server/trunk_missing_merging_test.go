package server_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// trunkDeletedMidRun is a one-ticket epic whose trunk develop is deleted while
// the agent works. It returns develop's tip and a count of chat text sent.
func trunkDeletedMidRun(t *testing.T, autoMerge bool) (f *schedFixture, develop string, chatCount func(string) int) {
	t.Helper()
	chatCfg, chatCount := chatRecorder(t)
	f = newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
		f.git(t, "branch", "-D", "develop")
	}, chatCfg)
	f.git(t, "branch", "develop")
	if autoMerge {
		servertest.SetProjectTrunk(t, f.store, "proj", f.repo, "develop")
	} else {
		servertest.SetProjectTrunkNoAutoMerge(t, f.store, "proj", f.repo, "develop")
	}
	return f, f.rev(t, "develop"), chatCount
}

func TestTrunkMissingAtMerging_TicketMergesOnItsRootAndTheRootPauses(t *testing.T) {
	t.Parallel()
	f, develop, chatCount := trunkDeletedMidRun(t, true)
	f.runEpic(t, server.EventRootPaused)

	if body := readFile(t, f.ticketFile(t, "01")); !strings.Contains(body, "status: done") {
		t.Errorf("ticket did not merge on its root's branch:\n%s", body)
	}
	if got := f.git(t, "show", schedEpic+":01.txt"); got != "01" {
		t.Errorf("%s:01.txt = %q, want the ticket's work", schedEpic, got)
	}
	if got := f.rev(t, "main"); got != develop {
		t.Errorf("main moved to %s", got)
	}
	if waitForChat(chatCount, "the trunk branch is missing: project") == 0 {
		t.Errorf("no root pause with reason trunk-missing reached chat")
	}
	if slices.Contains(f.h.Server.LockedTrunks(), "") {
		t.Errorf("a merge locked on an empty target")
	}
}

func TestTrunkMissingAtMerging_AutoMergeOffIsNotPaused(t *testing.T) {
	t.Parallel()
	f, _, _ := trunkDeletedMidRun(t, false)
	f.runEpic(t, server.EventRootCompleted)

	if body := readFile(t, f.ticketFile(t, "01")); !strings.Contains(body, "status: done") {
		t.Errorf("ticket not done:\n%s", body)
	}
	if slices.Contains(f.h.Server.LockedTrunks(), "") {
		t.Errorf("a merge locked on an empty target")
	}
}
