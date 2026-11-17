package chatdrafts_test

import (
	"os"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/herdrfake"
)

// TestMain re-enters the test binary as the session process and the fake
// herdr; without both calls the binary re-runs the suite in every child and
// forks without end.
func TestMain(m *testing.M) {
	servertest.RunSessionProcesses()
	herdrfake.RunHelperProcess()
	code := m.Run()
	servertest.RemoveFakeCodex()
	os.Exit(code)
}
