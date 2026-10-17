package e2e

import (
	"fmt"
	"os"
	"testing"
)

// binRoot holds every binary the tests build, so one RemoveAll after the run
// frees them all; the builders sit behind sync.Once, past any test's Cleanup.
var binRoot string

func TestMain(m *testing.M) {
	var err error
	binRoot, err = os.MkdirTemp("", "gx-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(binRoot)
	os.Exit(code)
}
