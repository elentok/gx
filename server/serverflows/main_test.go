package serverflows_test

import (
	"os"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/herdrfake"
)

const testFlushInterval = servertest.FlushInterval

func TestMain(m *testing.M) {
	servertest.RunSessionProcesses()
	herdrfake.RunHelperProcess()
	// One zone away from UTC for every test: a test that swapped it mid-run
	// moved the budget day under its parallel neighbours.
	time.Local = time.FixedZone("test", 5*3600)
	code := m.Run()
	servertest.RemoveFakeCodex()
	os.Exit(code)
}
