package prompt_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

// Aliases for the helpers that live in servertest, so the callers here keep
// their short names.
const (
	runEndText     = servertest.RunEndText
	countdownDelay = servertest.CountdownDelay
)

type grillingFixture = servertest.GrillingFixture

var (
	readMeta               = servertest.ReadMeta
	report                 = servertest.Report
	screenHas              = servertest.ScreenHas
	waitForHolder          = servertest.WaitForHolder
	waitForScreen          = servertest.WaitForScreen
	waitForSession         = servertest.WaitForSession
	waitUntil              = servertest.WaitUntil
	storeWithMapStep       = servertest.StoreWithMapStep
	writeAssistantMessage  = servertest.WriteAssistantMessage
	clearByInput           = servertest.ClearByInput
	startAdvancing         = servertest.StartAdvancing
	endTurn                = servertest.EndTurn
	sessionSnapshot        = servertest.SessionSnapshot
	autoSends              = servertest.AutoSends
	startWebDesign         = servertest.StartWebDesign
	webSessionWrite        = servertest.WebSessionWrite
	sessionToken           = servertest.SessionToken
	startFakeSessionWith   = servertest.StartFakeSessionWith
	startGrillingSessionIn = servertest.StartGrillingSessionIn
	submitted              = servertest.Submitted
	waitForSubmitted       = servertest.WaitForSubmitted
	waitAlerts             = servertest.WaitAlerts
	withAlerts             = servertest.WithAlerts
	writeTranscript        = servertest.WriteTranscript
)

// designsFixture wraps servertest.DesignsFixture with lower-case fields.
type designsFixture struct {
	h    *servertest.Harness
	ctx  context.Context
	evs  <-chan wire.Event
	repo string
	base string
	ck   *http.Cookie
}

func fromShared(d servertest.DesignsFixture) designsFixture {
	return designsFixture{h: d.H, ctx: d.Ctx, evs: d.Evs, repo: d.Repo, base: d.Base, ck: d.Ck}
}

// shared is the servertest fixture the shared methods run on.
func (f designsFixture) shared() servertest.DesignsFixture {
	return servertest.DesignsFixture{H: f.h, Ctx: f.ctx, Evs: f.evs, Repo: f.repo, Base: f.base, Ck: f.ck}
}

func sessionSnapshotOf(t *testing.T, f designsFixture, id string) wire.SessionInfo {
	t.Helper()
	return servertest.SessionSnapshotOf(t, f.shared(), id)
}
