package servertest

import (
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
)

// CountdownDelay is the auto-advance delay StartAdvancing runs with.
const CountdownDelay = time.Second

// StartAdvancing starts a grilling session on the map epic with the
// auto-advance countdown set to CountdownDelay.
func StartAdvancing(t *testing.T, store string) *GrillingFixture {
	t.Helper()
	h, ctx, evs, id, conv, path := StartGrillingSessionIn(t, "proj:map", store, WithAutoAdvanceDelay(CountdownDelay))
	return &GrillingFixture{H: h, Ctx: ctx, Evs: evs, ID: id, Conv: conv, Path: path}
}

// SessionSnapshot is the fixture session's entry in the sessions list.
func SessionSnapshot(t *testing.T, f *GrillingFixture) wire.SessionInfo {
	t.Helper()
	snap, err := f.H.Client.Sessions(f.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range snap {
		if s.ID == f.ID {
			return s
		}
	}
	t.Fatalf("session %s not in the snapshot", f.ID)
	return wire.SessionInfo{}
}

// AutoSends is how many Continues in a row the server sent the fixture session.
func AutoSends(t *testing.T, f *GrillingFixture) int {
	t.Helper()
	n, _ := ReadMeta(t, f.H, f.ID)["auto_sends"].(float64)
	return int(n)
}

// OpenNew opens a design for a new epic slug with the given idea.
func (f DesignsFixture) OpenNew(t *testing.T, slug, idea string) wire.SessionResult {
	t.Helper()
	res, err := f.H.Client.OpenDesign(f.Ctx, wire.DesignOpenRequest{Epic: slug, NewDesign: true, Idea: idea, Project: "proj"})
	if err != nil {
		t.Fatal(err)
	}
	return res
}
