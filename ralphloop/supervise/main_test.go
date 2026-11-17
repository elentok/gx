package supervise

import (
	"fmt"
	"os"
	"testing"

	"github.com/elentok/gx/ralphloop/internal/fakes"
	"go.uber.org/goleak"
)

// goleak.VerifyTestMain fails the specific leaking test at leak time (stack
// trace and all) instead of letting a stuck goroutine surface as an
// unrelated multi-minute package hang.
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
