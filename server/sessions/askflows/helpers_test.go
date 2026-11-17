package askflows_test

import (
	"os"
	"testing"

	"github.com/elentok/gx/server/servertest"
)

// The servertest harness runs this test binary as its session holder;
// without the call the binary re-runs the suite in every child and forks
// without end.
func TestMain(m *testing.M) {
	servertest.RunSessionProcesses()
	code := m.Run()
	removeGxBin()
	os.Exit(code)
}

const (
	answerAddr = servertest.AnswerAddr
	schedEpic  = servertest.SchedEpic
)

var (
	waitFor          = servertest.Await
	startFakeSession = servertest.StartFakeSession
	waitForHolder    = servertest.WaitForHolder
	waitForScreen    = servertest.WaitForScreen
	waitForSession   = servertest.WaitForSession
)
