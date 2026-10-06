package cmd

import (
	"os"
	"testing"
)

// TestMain keeps land locks (which live in the state dir) off the machine's
// real one.
func TestMain(m *testing.M) {
	state, err := os.MkdirTemp("", "gx-cmd-state")
	if err != nil {
		panic(err)
	}
	os.Setenv("XDG_STATE_HOME", state)
	code := m.Run()
	os.RemoveAll(state)
	os.Exit(code)
}
