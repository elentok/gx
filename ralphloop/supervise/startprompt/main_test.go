package startprompt

import (
	"fmt"
	"os"
	"testing"

	"github.com/elentok/gx/ralphloop/internal/fakes"
	"github.com/elentok/gx/testutil/herdrfake"
	"go.uber.org/goleak"
)

// TestMain re-enters the test binary as the fake herdr helper when launched
// under herdrfake's coordinator (see herdrfake.RunHelperProcess), so this
// package's tests can put a hermetic fake `herdr` executable first in PATH.
// goleak fails the run when a goroutine leaks.
func TestMain(m *testing.M) {
	herdrfake.RunHelperProcess()
	state, err := os.MkdirTemp("", "gx-ralphloop-state")
	if err != nil {
		panic(err)
	}
	fakes.StateDir = state
	code := m.Run()
	os.RemoveAll(state)
	if code == 0 {
		if err := goleak.Find(); err != nil {
			fmt.Fprintf(os.Stderr, "goleak: %v\n", err)
			code = 1
		}
	}
	os.Exit(code)
}
