package server_test

import (
	"context"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

const countdownDelay = servertest.CountdownDelay

// grillingFixture is the root view of servertest.GrillingFixture, with the
// lowercase fields its many callers use.
type grillingFixture struct {
	h    *servertest.Harness
	ctx  context.Context
	evs  <-chan wire.Event
	id   string
	conv string
	path string
}

func (f *grillingFixture) shared() *servertest.GrillingFixture {
	return &servertest.GrillingFixture{H: f.h, Ctx: f.ctx, Evs: f.evs, ID: f.id, Conv: f.conv, Path: f.path}
}

func endTurn(t *testing.T, f *grillingFixture) {
	t.Helper()
	servertest.EndTurn(t, f.shared())
}

func startAdvancing(t *testing.T, store string) *grillingFixture {
	t.Helper()
	g := servertest.StartAdvancing(t, store)
	return &grillingFixture{g.H, g.Ctx, g.Evs, g.ID, g.Conv, g.Path}
}

func sessionSnapshot(t *testing.T, f *grillingFixture) wire.SessionInfo {
	t.Helper()
	return servertest.SessionSnapshot(t, f.shared())
}

func autoSends(t *testing.T, f *grillingFixture) int {
	t.Helper()
	return servertest.AutoSends(t, f.shared())
}
