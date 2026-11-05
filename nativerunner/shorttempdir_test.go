package nativerunner_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/nativerunner/holder"
)

// shortTempDir keeps holder.sock under the unix socket path limit, which a
// t.TempDir path can exceed on macOS.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "gxh")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting until %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitForScreen(t *testing.T, c *holder.HolderClient, want string) {
	t.Helper()
	waitFor(t, "the screen shows "+want, func() bool {
		rows, _, err := c.Screen(10)
		return err == nil && strings.Contains(strings.Join(rows, "\n"), want)
	})
}
