package epics_test

import (
	"github.com/elentok/gx/server/wire"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

func TestEpicNotice_MergeSendsEpicMerged(t *testing.T) {
	t.Parallel()
	chat, count := chatRecorder(t)
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
	}, chat)
	f.RunEpic(t, wire.EventRootCompleted)

	if got := waitForChat(count, epicMergedText); got != 1 {
		t.Errorf("%q sent %d times, want once", epicMergedText, got)
	}
	if got := count(epicNotMergedText); got != 0 {
		t.Errorf("%q sent %d times, want none", epicNotMergedText, got)
	}
}

func TestEpicNotice_AutoFFMergeOffSendsNotMerged(t *testing.T) {
	t.Parallel()
	chat, count := chatRecorder(t)
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
	}, chat)
	servertest.SetProjectRepoNoAutoMerge(t, f.Store, "proj", f.Repo)
	f.RunEpic(t, wire.EventRootCompleted)

	if got := waitForChat(count, epicNotMergedText); got != 1 {
		t.Errorf("%q sent %d times, want once", epicNotMergedText, got)
	}
	if got := count(epicMergedText); got != 0 {
		t.Errorf("%q sent %d times, want none", epicMergedText, got)
	}
}

func TestEpicNotice_NeedsRebasePauseSendsNotMerged(t *testing.T) {
	t.Parallel()
	chat, count := chatRecorder(t)
	f := newSchedFixture(t, atMergeCap(map[string]servertest.TicketOpts{"01": {}}), func(f *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
		testutil.WriteFile(t, f.Repo, "moved.txt", "moved")
		testutil.CommitAll(t, f.Repo, "main moves on")
	}, chat)
	f.RunEpic(t, wire.EventRootPaused)

	if got := waitForChat(count, epicNotMergedText); got != 1 {
		t.Errorf("%q sent %d times, want once", epicNotMergedText, got)
	}
	if got := count(epicMergedText); got != 0 {
		t.Errorf("%q sent %d times, want none", epicMergedText, got)
	}
}
