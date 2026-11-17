package chatdrafts_test

import (
	"os"
	"testing"

	"github.com/elentok/gx/server/servertest"
)

// TestMain re-enters the test binary as the session process; without the call
// the binary re-runs the suite in every child and forks without end.
func TestMain(m *testing.M) {
	servertest.RunSessionProcesses()
	code := m.Run()
	servertest.RemoveFakeCodex()
	os.Exit(code)
}
