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

// OpenNew opens a design for a new epic slug with the given idea.
func (f DesignsFixture) OpenNew(t *testing.T, slug, idea string) wire.SessionResult {
	t.Helper()
	res, err := f.H.Client.OpenDesign(f.Ctx, wire.DesignOpenRequest{Epic: slug, NewDesign: true, Idea: idea, Project: "proj"})
	if err != nil {
		t.Fatal(err)
	}
	return res
}
