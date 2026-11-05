package banners_test

import (
	"os"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
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
	testFlushInterval = servertest.FlushInterval
)

type slackHook = rigs.SlackHook

var (
	report                 = servertest.Report
	readMeta               = servertest.ReadMeta
	waitForSession         = servertest.WaitForSession
	waitForHolder          = servertest.WaitForHolder
	waitForSubmitted       = servertest.WaitForSubmitted
	waitForScreen          = servertest.WaitForScreen
	writeTranscript        = servertest.WriteTranscript
	startFakeSessionWith   = servertest.StartFakeSessionWith
	stopFakeSession        = servertest.StopFakeSession
	failureNow             = rigs.FailureNow
	waitForFailure         = rigs.WaitForFailure
	waitFor                = servertest.Await
	newSlackHook           = rigs.NewSlackHook
	idleWithAction         = servertest.IdleWithAction
	withAlerts             = servertest.WithAlerts
	startBannerSession     = servertest.StartBannerSession
	reach                  = servertest.Reach
	waitAlerts             = servertest.WaitAlerts
	openTUI                = servertest.OpenTUI
	reportFocus            = servertest.ReportFocus
	startGrillingSessionIn = servertest.StartGrillingSessionIn
	writeAssistantMessage  = servertest.WriteAssistantMessage
)
