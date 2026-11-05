package fixagent_test

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/herdrfake"
)

// The servertest harness runs this test binary as its session holder and as
// the fake herdr; without both calls the binary re-runs the suite in every
// child and forks without end.
func TestMain(m *testing.M) {
	servertest.RunSessionProcesses()
	herdrfake.RunHelperProcess()
	os.Exit(m.Run())
}

const repairToken = servertest.RepairToken

var (
	readMeta        = servertest.ReadMeta
	stopFakeSession = servertest.StopFakeSession
	reload          = servertest.Reload
)

type designsFixture struct {
	h    *servertest.Harness
	ctx  context.Context
	evs  <-chan wire.Event
	repo string
	base string
	ck   *http.Cookie
}

// startDesignsIn starts a server on a store the caller has already seeded, with
// the web mux up.
func startDesignsIn(t *testing.T, store string, opts ...func(*server.Config)) designsFixture {
	t.Helper()
	d := servertest.StartDesignsIn(t, store, opts...)
	return designsFixture{h: d.H, ctx: d.Ctx, evs: d.Evs, repo: d.Repo, base: d.Base, ck: d.Ck}
}

// shared is the servertest fixture the shared methods run on.
func (f designsFixture) shared() servertest.DesignsFixture {
	return servertest.DesignsFixture{H: f.h, Ctx: f.ctx, Evs: f.evs, Repo: f.repo, Base: f.base, Ck: f.ck}
}

func (f designsFixture) live(t *testing.T) []wire.SessionInfo {
	t.Helper()
	return f.shared().Live(t)
}
