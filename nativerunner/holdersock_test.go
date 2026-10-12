package nativerunner_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/nativerunner"
)

// echoChild prints a ready line, then echoes every line it reads.
const echoChild = `printf 'ready\n'; while read -r l; do printf 'got:%s\n' "$l"; done`

// startHolder runs a holder over child in-process and dials it.
func startHolder(t *testing.T, child string) (dir string, c *nativerunner.HolderClient, done <-chan error) {
	t.Helper()
	dir = shortTempDir(t)
	errc := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		errc <- nativerunner.RunHolder(dir, []string{"sh", "-c", child})
	}()
	waitFor(t, "holder.sock answers", func() bool {
		var err error
		c, err = nativerunner.DialHolder(dir)
		return err == nil
	})
	t.Cleanup(func() {
		_ = c.Stop()
		<-finished
	})
	return dir, c, errc
}

func TestHolderSocket_CoreOps(t *testing.T) {
	nativerunner.SetHolderOptionalOps(t, []string{"deny-reason"})
	dir, c, done := startHolder(t, echoChild)

	if c.Version() != nativerunner.HolderProtocolVersion {
		t.Errorf("version = %d; want %d", c.Version(), nativerunner.HolderProtocolVersion)
	}
	if !c.Supports("deny-reason") {
		t.Error("advertised optional op reported unsupported")
	}
	waitForScreen(t, c, "ready")
	if err := c.SendKeys("hi\r"); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, c, "got:hi")

	if err := c.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("holder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, nativerunner.ExitCodeFile)); err != nil {
		t.Errorf("no exit code after stop: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, nativerunner.HolderSocketFile)); !os.IsNotExist(err) {
		t.Errorf("holder.sock left behind: %v", err)
	}
}

func TestHolderSocket_NoOptionalOps(t *testing.T) {
	_, c, _ := startHolder(t, echoChild)

	if c.Supports("deny-reason") {
		t.Error("holder with no optional ops reports one supported")
	}
	waitForScreen(t, c, "ready")
}

func waitForScreen(t *testing.T, c *nativerunner.HolderClient, want string) {
	t.Helper()
	waitFor(t, "the screen shows "+want, func() bool {
		rows, _, err := c.Screen(10)
		return err == nil && strings.Contains(strings.Join(rows, "\n"), want)
	})
}

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
