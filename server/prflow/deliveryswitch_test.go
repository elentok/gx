package prflow_test

import (
	"testing"

	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/ghfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

const (
	deliveryPR    = `,"delivery":"pr"`
	deliveryMerge = `,"delivery":"merge"`
)

func epicState(f *rigs.PRFlow) wire.EpicState {
	return f.H.Server.EpicState(epicRoot, tickets.LoadEpicAt(f.EpicDir()))
}

func wantState(t *testing.T, f *rigs.PRFlow, want wire.EpicState) {
	t.Helper()
	if got := epicState(f); got != want {
		t.Fatalf("epic state = %q, want %q", got, want)
	}
}

func startText(t *testing.T, f *rigs.PRFlow) string {
	t.Helper()
	id, _, err := prflow.StartPRText(f.EpicDir(), prflow.TextJob{Epic: "proj:epic-a", Worktree: "/wt"})
	if err != nil {
		t.Fatal(err)
	}
	f.Rescan(t)
	return id
}

func textStatus(t *testing.T, f *rigs.PRFlow) string {
	t.Helper()
	all := prTextTickets(f)
	if len(all) != 1 {
		t.Fatalf("pr-text tickets = %d", len(all))
	}
	return all[0].Status
}

func TestDeliverySwitch_PRToMergeMakesAwaitingPRAwaitingMerge(t *testing.T) {
	f := openRig(t, "")
	f.Rescan(t)
	wantState(t, f, wire.EpicStateAwaitingPR)
	setProject(t, f, deliveryMerge)
	f.Tick(t)
	wantState(t, f, wire.EpicStateAwaitingMerge)
}

func TestDeliverySwitch_MergeToPRMakesAwaitingMergeAwaitingPR(t *testing.T) {
	f := openRig(t, "")
	setProject(t, f, deliveryMerge)
	f.Rescan(t)
	wantState(t, f, wire.EpicStateAwaitingMerge)
	setProject(t, f, deliveryPR)
	f.Tick(t)
	wantState(t, f, wire.EpicStateAwaitingPR)
}

func TestDeliverySwitch_PRToMergeCancelsPRTextAndOpensNoPR(t *testing.T) {
	f := openRig(t, "")
	startText(t, f)
	wantState(t, f, wire.EpicStateWritingPRText)
	setProject(t, f, deliveryMerge)
	f.Tick(t)
	if got := textStatus(t, f); got != string(schema.StatusCancelled) {
		t.Errorf("pr-text status = %q, want cancelled", got)
	}
	wantState(t, f, wire.EpicStateAwaitingMerge)
	f.Tick(t)
	if creates(f) != 0 {
		t.Errorf("pr create calls = %d", creates(f))
	}
}

func TestDeliverySwitch_EpicWithAPRKeepsItAfterSwitchToMerge(t *testing.T) {
	f := openRig(t, "")
	openPR(f)
	f.StampPR(t, 5, prURL)
	f.Tick(t)
	setProject(t, f, deliveryMerge)
	f.GH.Update(func(s *ghfake.State) { s.PRs[0].HeadSHA = "sha2" })
	f.Tick(t)
	v, ok := f.H.Server.PR.Watcher().View("proj", "epic-a")
	if !ok || v.HeadSHA != "sha2" {
		t.Fatalf("view = %+v, %v; want the PR still polled", v, ok)
	}
	// PR open is never Awaiting merge, so the plain Merge action is not offered.
	wantState(t, f, wire.EpicStatePROpen)
}

func TestDeliverySwitch_AutoPROffLetsARunningPRTextFinish(t *testing.T) {
	f := openRig(t, "")
	setProject(t, f, autoOn)
	startText(t, f)
	setProject(t, f, deliveryPR)
	f.Tick(t)
	if got := textStatus(t, f); got != string(schema.StatusOpen) {
		t.Errorf("pr-text status = %q, want it left alone", got)
	}
	wantState(t, f, wire.EpicStateWritingPRText)
}
