package infoflows_test

import (
	"os"
	"testing"

	"github.com/elentok/gx/server/servertest"
)

// The servertest harness runs this test binary as its session holder and as
// the fake herdr; without both calls the binary re-runs the suite in every
// child and forks without end.
func TestMain(m *testing.M) {
	servertest.RunSessionProcesses()
	os.Exit(m.Run())
}
