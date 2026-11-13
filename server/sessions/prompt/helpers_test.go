package prompt_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/elentok/gx/server/servertest/rigs"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

// Aliases for the helpers that live in servertest, so the callers here keep
// their short names.
const (
	runEndText     = rigs.RunEndText
	countdownDelay = rigs.CountdownDelay
)

type grillingFixture = rigs.GrillingFixture

var (
	readMeta               = servertest.ReadMeta
	report                 = servertest.Report
	screenHas              = servertest.ScreenHas
	waitForHolder          = servertest.WaitForHolder
	waitForScreen          = servertest.WaitForScreen
	waitForSession         = servertest.WaitForSession
	waitUntil              = servertest.WaitUntil
	storeWithMapStep       = rigs.StoreWithMapStep
	writeAssistantMessage  = rigs.WriteAssistantMessage
	clearByInput           = rigs.ClearByInput
	startAdvancing         = rigs.StartAdvancing
	endTurn                = rigs.EndTurn
	sessionSnapshot        = rigs.SessionSnapshot
	autoSends              = rigs.AutoSends
	startWebDesign         = rigs.StartWebDesign
	webSessionWrite        = rigs.WebSessionWrite
	sessionToken           = rigs.SessionToken
	startFakeSession       = servertest.StartFakeSession
	idleWithAction         = servertest.IdleWithAction
	startFakeSessionWith   = servertest.StartFakeSessionWith
	startGrillingSessionIn = rigs.StartGrillingSessionIn
	submitted              = servertest.Submitted
	waitForSubmitted       = servertest.WaitForSubmitted
	waitAlerts             = servertest.WaitAlerts
	withAlerts             = servertest.WithAlerts
	writeTranscript        = servertest.WriteTranscript
)

// designsFixture wraps rigs.DesignsFixture with lower-case fields.
type designsFixture struct {
	h    *servertest.Harness
	ctx  context.Context
	evs  <-chan wire.Event
	repo string
	base string
	ck   *http.Cookie
}

func fromShared(d rigs.DesignsFixture) designsFixture {
	return designsFixture{h: d.H, ctx: d.Ctx, evs: d.Evs, repo: d.Repo, base: d.Base, ck: d.Ck}
}

// shared is the servertest fixture the shared methods run on.
func (f designsFixture) shared() rigs.DesignsFixture {
	return rigs.DesignsFixture{H: f.h, Ctx: f.ctx, Evs: f.evs, Repo: f.repo, Base: f.base, Ck: f.ck}
}

func (f promptFixture) answer(t *testing.T, req wire.SessionAnswerRequest) wire.SessionResult {
	t.Helper()
	req.ID = f.id
	res, err := f.h.Client.Sessions.Answer(f.ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func sessionSnapshotOf(t *testing.T, f designsFixture, id string) wire.SessionInfo {
	t.Helper()
	return rigs.SessionSnapshotOf(t, f.shared(), id)
}
