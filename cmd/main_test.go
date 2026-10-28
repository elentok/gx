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
	os.Exit(m.Run())
}
