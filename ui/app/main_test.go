package app

import (
	"os"
	"testing"
)

// TestMain points HOME at a scratch directory for the whole package's test
// binary. This package's tests build the real app (New(...)), which reaches
// real, unexported state/config paths several layers down (e.g.
// tickets.LoadQueueStore, ralphloop.NotificationGate) that have no override
// hook reachable from outside their own package - config.UserConfigDir/
// UserStateDir both resolve from os.UserHomeDir(), so redirecting HOME here
// is what keeps every one of those real writes off the developer machine's
// actual ~/.config/gx and ~/.local/state/gx.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gx-ui-app-home")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	os.Setenv("HOME", dir)
	os.Exit(m.Run())
}
