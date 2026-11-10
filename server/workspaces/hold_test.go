package workspaces_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
)

// holdFixture is a Design proj:map with a ready, unqueued proj:map-impl, and a
// plain loop epic proj:plain.
func holdFixture(t *testing.T) servertest.DesignsFixture {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "plain", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rigs.WriteEpicFiles(t, store, "map-impl", "---\nstatus: open\n---\n", map[string]string{"01-a.md": rigs.IssueMD("01", "open", "", "A")})
	return servertest.StartDesignsIn(t, store)
}

func liveIDs(t *testing.T, f servertest.DesignsFixture) []string {
	t.Helper()
	var ids []string
	for _, s := range f.Live(t) {
		ids = append(ids, s.ID)
	}
	return ids
}

func TestHold_LiveDesignSessionNeedsYesThenExits(t *testing.T) {
	for _, addr := range []string{"proj:map", "proj:map-impl"} {
		t.Run(addr, func(t *testing.T) {
			f := holdFixture(t)
			id := f.Open(t, "proj:map", false).Session.ID

			res := hold(t, f, false, false, addr)
			if r := res.Results[0]; !r.Refused() || r.Reason != wire.ReasonNeedsYes || !strings.Contains(r.Message, id) || !slices.Equal(r.Sessions, []string{id}) {
				t.Fatalf("hold without yes = %+v, want a refusal naming %s", res, id)
			}
			if !slices.Contains(liveIDs(t, f), id) {
				t.Fatal("the refused hold ended the session")
			}

			res = hold(t, f, true, false, addr)
			if res.Results[0].Refused() {
				t.Fatalf("hold with yes = %+v", res)
			}
			if !slices.Equal(res.Results[0].Sessions, []string{id}) || !slices.Equal(res.Held, []string{"proj:map"}) {
				t.Errorf("hold with yes = %+v, want session %s ended and proj:map held", res, id)
			}
			if slices.Contains(liveIDs(t, f), id) {
				t.Error("the session is still live after hold with yes")
			}
			snap, err := f.H.Client.Snapshot(f.Ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(snap.Held, []string{"proj:map"}) {
				t.Errorf("snapshot held = %v, want proj:map", snap.Held)
			}
			servertest.WaitForEvent(f.Ctx, t, f.Evs, wire.EventWorkspacesChanged, "")
		})
	}
}

func TestHold_UnholdAsksNothingAndStartsNothing(t *testing.T) {
	f := holdFixture(t)
	if res := hold(t, f, false, false, "proj:map"); res.Results[0].Refused() {
		t.Fatalf("hold = %+v", res)
	}
	res, err := f.H.Client.Admin.UnholdEpics(f.Ctx, []string{"proj:map-impl"})
	if err != nil || res.Results[0].Refused() || len(res.Held) != 0 {
		t.Fatalf("unhold = %+v, %v, want nothing held", res, err)
	}
	if live := liveIDs(t, f); len(live) != 0 {
		t.Errorf("unhold started sessions %v", live)
	}
}

func TestHold_UnholdRefusesTargetNotOnHold(t *testing.T) {
	f := holdFixture(t)
	if res := hold(t, f, false, false, "proj:map"); res.Results[0].Refused() {
		t.Fatalf("hold = %+v", res)
	}
	res, err := f.H.Client.Admin.UnholdEpics(f.Ctx, []string{"proj:map", "proj:plain"})
	if err != nil {
		t.Fatal(err)
	}
	if r := res.Results[0]; r.Refused() || r.Status != wire.TargetDone {
		t.Errorf("held target = %+v, want done", r)
	}
	if r := res.Results[1]; !r.Refused() || r.Reason != wire.ReasonNotOnHold || r.Message != "not on hold" {
		t.Errorf("not-held target = %+v, want refused not-on-hold", r)
	}
}

func TestHold_QueuedLoopEpicAndQueuedImplAreRefused(t *testing.T) {
	f := holdFixture(t)
	if res, err := f.H.Client.Queue.Add(f.Ctx, "proj:plain", ""); err != nil || res.Refused {
		t.Fatalf("queue add plain = %+v, %v", res, err)
	}
	if res, err := f.H.Client.Queue.Add(f.Ctx, "proj:map-impl", ""); err != nil || res.Refused {
		t.Fatalf("queue add map-impl = %+v, %v", res, err)
	}
	for _, addr := range []string{"proj:plain", "proj:map", "proj:map-impl"} {
		res := hold(t, f, true, false, addr)
		if r := res.Results[0]; !r.Refused() || r.Reason != wire.ReasonHoldQueued || r.Message != "Remove it from the queue first" || len(res.Held) != 0 {
			t.Errorf("hold %s = %+v, want the queue refusal", addr, res)
		}
	}
}

func TestHold_RefusesOneOffsFollowUpsAndUnknown(t *testing.T) {
	f := holdFixture(t)
	for addr, reason := range map[string]string{
		"proj:one-offs":   wire.ReasonHoldNotHoldable,
		"proj:follow-ups": wire.ReasonHoldNotHoldable,
		"proj:nope":       wire.ReasonHoldUnknown,
		"nocolon":         wire.ReasonHoldUnknown,
	} {
		res := hold(t, f, true, false, addr)
		if r := res.Results[0]; !r.Refused() || r.Reason != reason || r.Target != addr {
			t.Errorf("hold %s = %+v, want refusal %s", addr, res, reason)
		}
	}
}

func hold(t *testing.T, f servertest.DesignsFixture, yes, dryRun bool, targets ...string) wire.HoldResult {
	t.Helper()
	res, err := f.H.Client.Admin.HoldEpics(f.Ctx, targets, yes, dryRun)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) != len(targets) {
		t.Fatalf("results = %+v, want one per target %v", res.Results, targets)
	}
	return res
}

func TestHold_ManyTargetsInOneCall(t *testing.T) {
	f := holdFixture(t)
	res := hold(t, f, false, false, "proj:plain", "proj:map-impl")
	for _, r := range res.Results {
		if r.Status != wire.TargetDone {
			t.Errorf("result %+v, want done", r)
		}
	}
	if res.Results[1].Target != "proj:map-impl" || !slices.Equal(res.Held, []string{"proj:plain", "proj:map"}) {
		t.Errorf("hold = %+v, want results in argument order and plain + map held", res)
	}
}

func TestHold_QueuedTargetIsRefusedBesideADoneOne(t *testing.T) {
	f := holdFixture(t)
	if res, err := f.H.Client.Queue.Add(f.Ctx, "proj:plain", ""); err != nil || res.Refused {
		t.Fatalf("queue add = %+v, %v", res, err)
	}
	res := hold(t, f, false, false, "proj:plain", "proj:map")
	if !res.Results[0].Refused() || res.Results[1].Status != wire.TargetDone || !slices.Equal(res.Held, []string{"proj:map"}) {
		t.Errorf("hold = %+v, want plain refused and map held", res)
	}
}

func TestHold_DryRunHoldsNothingAndListsSessions(t *testing.T) {
	f := holdFixture(t)
	id := f.Open(t, "proj:map", false).Session.ID
	res := hold(t, f, false, true, "proj:map", "proj:plain")
	if r := res.Results[0]; r.Status != wire.TargetFits || !slices.Equal(r.Sessions, []string{id}) {
		t.Errorf("dry run of the live Design = %+v, want fits listing %s", r, id)
	}
	if res.Results[1].Status != wire.TargetFits || len(res.Held) != 0 {
		t.Errorf("dry run = %+v, want plain fits and nothing held", res)
	}
	if !slices.Contains(liveIDs(t, f), id) {
		t.Error("the dry run ended the session")
	}
}

func TestHold_NeedsYesRefusesOnlyTheLiveDesign(t *testing.T) {
	f := holdFixture(t)
	id := f.Open(t, "proj:map", false).Session.ID
	res := hold(t, f, false, false, "proj:map", "proj:plain")
	if r := res.Results[0]; r.Reason != wire.ReasonNeedsYes || !slices.Equal(r.Sessions, []string{id}) {
		t.Errorf("live Design = %+v, want needs-yes", r)
	}
	if res.Results[1].Status != wire.TargetDone || !slices.Equal(res.Held, []string{"proj:plain"}) {
		t.Errorf("hold = %+v, want plain held beside the refusal", res)
	}
}
