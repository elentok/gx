package cmd

import (
	"os"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/claudeenv"
	"github.com/elentok/gx/testutil/herdrfake"
)

// TestMain keeps land locks (which live in the state dir) off the machine's
// real one, and serves the herdr fake's helper process.
func TestMain(m *testing.M) {
	servertest.RunSessionProcesses()
	herdrfake.RunHelperProcess()
	cleanup := claudeenv.Isolate()
	code := m.Run()
	cleanup()
	os.Exit(code)
}
