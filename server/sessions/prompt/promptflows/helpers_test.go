package promptflows_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

const firstPromptGrace = servertest.FirstPromptGrace

var (
	readMeta         = servertest.ReadMeta
	report           = servertest.Report
	submitted        = servertest.Submitted
	waitForHolder    = servertest.WaitForHolder
	waitForSession   = servertest.WaitForSession
	waitForSubmitted = servertest.WaitForSubmitted
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

func (f designsFixture) shared() servertest.DesignsFixture {
	return servertest.DesignsFixture{H: f.h, Ctx: f.ctx, Evs: f.evs, Repo: f.repo, Base: f.base, Ck: f.ck}
}

// promptFixture is a design session on a fake program with an input box,
// Idle, whose transcript lives under cfg.
type promptFixture struct {
	designsFixture
	id, conv, transcript string
}

func startPromptSession(t *testing.T) promptFixture {
	t.Helper()
	return fromPrompt(servertest.StartPromptSession(t, firstPromptGrace))
}

// fixture is the servertest fixture the shared methods run on.
func (f promptFixture) fixture() servertest.PromptFixture {
	return servertest.PromptFixture{DesignsFixture: f.shared(), ID: f.id, Conv: f.conv, Transcript: f.transcript}
}

func fromPrompt(p servertest.PromptFixture) promptFixture {
	return promptFixture{designsFixture: fromShared(p.DesignsFixture), id: p.ID, conv: p.Conv, transcript: p.Transcript}
}

func (f promptFixture) prompt(t *testing.T, text string) wire.SessionResult {
	t.Helper()
	return f.fixture().Prompt(t, text)
}
