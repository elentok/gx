package workspaces_test

import (
	"slices"
	"testing"

	"github.com/elentok/gx/server/wire"
)

func heldKeys(t *testing.T, held func() (wire.Snapshot, error)) []string {
	t.Helper()
	snap, err := held()
	if err != nil {
		t.Fatal(err)
	}
	return snap.Held
}

// Starting a Design session on a held Design takes the hold off (story 84).
func TestHold_OpeningAHeldDesignTakesTheHoldOff(t *testing.T) {
	f := holdFixture(t)
	f.Open(t, "proj:map", false)
	if res := hold(t, f, true, false, "proj:map"); res.Results[0].Refused() {
		t.Fatalf("hold = %+v", res)
	}
	snap := func() (wire.Snapshot, error) { return f.H.Client.Snapshot(f.Ctx) }
	if got := heldKeys(t, snap); !slices.Equal(got, []string{"proj:map"}) {
		t.Fatalf("held = %v, want proj:map", got)
	}
	if res := f.Open(t, "proj:map", false); res.Session == nil {
		t.Fatalf("open = %+v, want a session", res)
	}
	if got := heldKeys(t, snap); len(got) != 0 {
		t.Errorf("held after open = %v, want none", got)
	}
}

// Resuming an exited Design session takes the hold off too.
func TestHold_ResumingAHeldDesignSessionTakesTheHoldOff(t *testing.T) {
	f := holdFixture(t)
	id := f.Open(t, "proj:map", false).Session.ID
	if res := hold(t, f, true, false, "proj:map"); res.Results[0].Refused() {
		t.Fatalf("hold = %+v", res)
	}
	if res, err := f.H.Client.Sessions.Resume(f.Ctx, id); err != nil || res.Refused {
		t.Fatalf("resume = %+v, %v", res, err)
	}
	if got := heldKeys(t, func() (wire.Snapshot, error) { return f.H.Client.Snapshot(f.Ctx) }); len(got) != 0 {
		t.Errorf("held after resume = %v, want none", got)
	}
}
