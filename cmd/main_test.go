package cmd

import (
	"os"
	"testing"

	"github.com/elentok/gx/testutil/herdrfake"
)

// TestMain keeps land locks (which live in the state dir) off the machine's
// real one, and serves the herdr fake's helper process.
func TestMain(m *testing.M) {
	herdrfake.RunHelperProcess()
	state, err := os.MkdirTemp("", "gx-cmd-state")
	if err != nil {
		panic(err)
	}
	os.Setenv("XDG_STATE_HOME", state)
	code := m.Run()
	os.RemoveAll(state)
	os.Exit(code)
}
