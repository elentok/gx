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

			res, err := f.H.Client.Admin.HoldEpic(f.Ctx, addr, false)
			if err != nil {
				t.Fatal(err)
			}
			if !res.Refused || res.Reason != wire.ReasonHoldLiveSessions || !strings.Contains(res.Message, id) || !slices.Equal(res.Sessions, []string{id}) {
				t.Fatalf("hold without yes = %+v, want a refusal naming %s", res, id)
			}
			if !slices.Contains(liveIDs(t, f), id) {
				t.Fatal("the refused hold ended the session")
			}

			res, err = f.H.Client.Admin.HoldEpic(f.Ctx, addr, true)
			if err != nil || res.Refused {
				t.Fatalf("hold with yes = %+v, %v", res, err)
			}
			if !slices.Equal(res.Sessions, []string{id}) || !slices.Equal(res.Held, []string{"proj:map"}) {
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
	if res, err := f.H.Client.Admin.HoldEpic(f.Ctx, "proj:map", false); err != nil || res.Refused {
		t.Fatalf("hold = %+v, %v", res, err)
	}
	res, err := f.H.Client.Admin.UnholdEpic(f.Ctx, "proj:map-impl")
	if err != nil || res.Refused || len(res.Held) != 0 {
		t.Fatalf("unhold = %+v, %v, want nothing held", res, err)
	}
	if live := liveIDs(t, f); len(live) != 0 {
		t.Errorf("unhold started sessions %v", live)
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
		res, err := f.H.Client.Admin.HoldEpic(f.Ctx, addr, true)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Refused || res.Reason != wire.ReasonHoldQueued || res.Message != "Remove it from the queue first" || len(res.Held) != 0 {
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
		res, err := f.H.Client.Admin.HoldEpic(f.Ctx, addr, true)
		if err != nil || !res.Refused || res.Reason != reason {
			t.Errorf("hold %s = %+v, %v, want refusal %s", addr, res, err, reason)
		}
	}
}
