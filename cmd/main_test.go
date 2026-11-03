package cmd

import (
	"os"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/herdrfake"
)

// TestMain serves the session processes and the herdr fake's helper process.
// Tests get their dirs from envFor, so nothing here touches the process env.
func TestMain(m *testing.M) {
	servertest.RunSessionProcesses()
	herdrfake.RunHelperProcess()
	// Built before any test starts, so no test's time (or the slow-test limit)
	// pays for the cold compile. An error is kept and reported by sharedGx.
	_, _ = buildSharedGx()
	code := m.Run()
	if sharedGxDir != "" {
		_ = os.RemoveAll(sharedGxDir)
	}
	os.Exit(code)
}
