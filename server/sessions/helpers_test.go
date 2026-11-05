package sessions_test

import (
	"context"
	"github.com/elentok/gx/server/servertest/rigs"
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

// Aliases for the helpers that live in servertest, so the callers here keep
// their short names.
const (
	apiError401       = rigs.APIError401
	reply             = rigs.Reply
	turnDurationLine  = servertest.TurnDurationLine
	runEndText        = servertest.RunEndText
	repairToken       = servertest.RepairToken
	schedEpic         = servertest.SchedEpic
	testFlushInterval = servertest.FlushInterval
)

type slackHook = rigs.SlackHook

var (
	report                 = servertest.Report
	readMeta               = servertest.ReadMeta
	waitForHolder          = servertest.WaitForHolder
	waitForSession         = servertest.WaitForSession
	submitted              = servertest.Submitted
	waitForSubmitted       = servertest.WaitForSubmitted
	startFakeSession       = servertest.StartFakeSession
	startFakeSessionWith   = servertest.StartFakeSessionWith
	stopFakeSession        = servertest.StopFakeSession
	stoppedCodexSession    = rigs.StoppedCodexSession
	writeCodexRollout      = rigs.WriteCodexRollout
	writeTranscript        = servertest.WriteTranscript
	appendLines            = servertest.AppendLines
	appendTranscript       = servertest.AppendTranscript
	waitSessionState       = servertest.WaitSessionState
	failureNow             = rigs.FailureNow
	waitForFailure         = rigs.WaitForFailure
	setMapStep             = rigs.SetMapStep
	gitOut                 = servertest.GitOut
	waitFor                = servertest.Await
	reload                 = servertest.Reload
	newSlackHook           = rigs.NewSlackHook
	idleWithAction         = servertest.IdleWithAction
	withAlerts             = servertest.WithAlerts
	startBannerSession     = servertest.StartBannerSession
	reach                  = servertest.Reach
	waitAlerts             = servertest.WaitAlerts
	openTUI                = servertest.OpenTUI
	reportFocus            = servertest.ReportFocus
	screenHas              = servertest.ScreenHas
	waitForScreen          = servertest.WaitForScreen
	startGrillingSessionIn = servertest.StartGrillingSessionIn
	storeWithMapStep       = servertest.StoreWithMapStep
	writeAssistantMessage  = servertest.WriteAssistantMessage
	clearByInput           = servertest.ClearByInput
	startWebDesign         = rigs.StartWebDesign
	sessionToken           = rigs.SessionToken
	webSessionWrite        = rigs.WebSessionWrite
)

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
	return fromShared(servertest.StartDesigns(t, opts...))
}

// startDesignsIn is startDesigns on a store the caller has already seeded.
func startDesignsIn(t *testing.T, store string, opts ...func(*server.Config)) designsFixture {
	t.Helper()
	return fromShared(servertest.StartDesignsIn(t, store, opts...))
}

func fromShared(d servertest.DesignsFixture) designsFixture {
	return designsFixture{h: d.H, ctx: d.Ctx, evs: d.Evs, repo: d.Repo, base: d.Base, ck: d.Ck}
}

// shared is the servertest fixture the shared methods run on.
func (f designsFixture) shared() servertest.DesignsFixture {
	return servertest.DesignsFixture{H: f.h, Ctx: f.ctx, Evs: f.evs, Repo: f.repo, Base: f.base, Ck: f.ck}
}

func (f designsFixture) open(t *testing.T, epic string, isNew bool) wire.SessionResult {
	t.Helper()
	return f.shared().Open(t, epic, isNew)
}

func (f designsFixture) live(t *testing.T) []wire.SessionInfo {
	t.Helper()
	return f.shared().Live(t)
}

// schedFixture wraps servertest.SchedFixture so the callbacks receive the
// wrapper.
type schedFixture struct {
	*servertest.SchedFixture
}

func newSchedFixture(t *testing.T, tickets map[string]servertest.TicketOpts, agentTurn func(f *schedFixture, p *servertest.Turn, id string), cfgOpts ...func(*server.Config)) *schedFixture {
	t.Helper()
	f := &schedFixture{}
	var inner func(*servertest.SchedFixture, *servertest.Turn, string)
	if agentTurn != nil {
		inner = func(_ *servertest.SchedFixture, p *servertest.Turn, id string) { agentTurn(f, p, id) }
	}
	f.SchedFixture = servertest.NewSchedFixture(t, tickets, inner, cfgOpts...)
	return f
}
