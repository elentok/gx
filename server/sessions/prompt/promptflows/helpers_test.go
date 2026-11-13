package promptflows_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
)

const firstPromptGrace = rigs.FirstPromptGrace

const (
	runEndText     = rigs.RunEndText
	countdownDelay = rigs.CountdownDelay
)

type grillingFixture = rigs.GrillingFixture

var (
	readMeta               = servertest.ReadMeta
	report                 = servertest.Report
	submitted              = servertest.Submitted
	waitForHolder          = servertest.WaitForHolder
	waitForSession         = servertest.WaitForSession
	waitForSubmitted       = servertest.WaitForSubmitted
	screenHas              = servertest.ScreenHas
	waitForScreen          = servertest.WaitForScreen
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
	startGrillingSessionIn = rigs.StartGrillingSessionIn
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

func (f designsFixture) shared() rigs.DesignsFixture {
	return rigs.DesignsFixture{H: f.h, Ctx: f.ctx, Evs: f.evs, Repo: f.repo, Base: f.base, Ck: f.ck}
}

// promptFixture is a design session on a fake program with an input box,
// Idle, whose transcript lives under cfg.
type promptFixture struct {
	designsFixture
	id, conv, transcript string
}

func startPromptSession(t *testing.T) promptFixture {
	t.Helper()
	return fromPrompt(rigs.StartPromptSession(t, firstPromptGrace))
}

// fixture is the servertest fixture the shared methods run on.
func (f promptFixture) fixture() rigs.PromptFixture {
	return rigs.PromptFixture{DesignsFixture: f.shared(), ID: f.id, Conv: f.conv, Transcript: f.transcript}
}

func fromPrompt(p rigs.PromptFixture) promptFixture {
	return promptFixture{designsFixture: fromShared(p.DesignsFixture), id: p.ID, conv: p.Conv, transcript: p.Transcript}
}

func (f promptFixture) prompt(t *testing.T, text string) wire.SessionResult {
	t.Helper()
	return f.fixture().Prompt(t, text)
}
