package server_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/elentok/gx/server/servertest/rigs"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

// designsFixture wraps rigs.DesignsFixture for the session tests that
// start from a server with a map epic.
type designsFixture struct {
	h    *servertest.Harness
	ctx  context.Context
	evs  <-chan wire.Event
	repo string
	base string
	ck   *http.Cookie
}

// startDesigns starts a server with a map epic "map" and a plain epic "plain"
// in project proj, whose repo is the trunk checkout, with the web mux up.
func startDesigns(t *testing.T, opts ...func(*server.Config)) designsFixture {
	t.Helper()
	return fromShared(rigs.StartDesigns(t, opts...))
}

func fromShared(d rigs.DesignsFixture) designsFixture {
	return designsFixture{h: d.H, ctx: d.Ctx, evs: d.Evs, repo: d.Repo, base: d.Base, ck: d.Ck}
}

// shared is the servertest fixture the shared methods run on.
func (f designsFixture) shared() rigs.DesignsFixture {
	return rigs.DesignsFixture{H: f.h, Ctx: f.ctx, Evs: f.evs, Repo: f.repo, Base: f.base, Ck: f.ck}
}

func sessionSnapshotOf(t *testing.T, f designsFixture, id string) wire.SessionInfo {
	t.Helper()
	return rigs.SessionSnapshotOf(t, f.shared(), id)
}
