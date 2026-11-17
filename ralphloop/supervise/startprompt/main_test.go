package startprompt

import (
	"fmt"
	"os"
	"testing"

	"github.com/elentok/gx/ralphloop/internal/fakes"
	"go.uber.org/goleak"
)

// TestMain gives the tests a state dir; goleak fails the run when a goroutine
// leaks.
func TestMain(m *testing.M) {
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
