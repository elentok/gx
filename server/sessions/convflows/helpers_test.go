package convflows_test

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

const runEndText = rigs.RunEndText

var (
	readMeta              = servertest.ReadMeta
	report                = servertest.Report
	setMapStep            = rigs.SetMapStep
	startFakeSession      = servertest.StartFakeSession
	startFakeSessionWith  = servertest.StartFakeSessionWith
	stopFakeSession       = servertest.StopFakeSession
	waitForHolder         = servertest.WaitForHolder
	waitForScreen         = servertest.WaitForScreen
	waitForSession        = servertest.WaitForSession
	writeAssistantMessage = rigs.WriteAssistantMessage
)
