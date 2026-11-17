package ralphloop

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/ralphloop/internal/fakes"
	"github.com/elentok/gx/tickets"
	"go.uber.org/goleak"
)

// TestMain gives merge locks a state dir.
// goleak.VerifyTestMain fails the specific leaking test at leak time instead
// of letting a stuck goroutine surface as a multi-minute package hang.
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

// addressTrailerValue is the trailer a ticket in writeEpic's layout merges with.
func addressTrailerValue(scratchDir, epicName, id string) string {
	a, _ := tickets.AddressOfPath(filepath.Join(scratchDir, epicName, "issues", id+"-x.md"), id)
	return a.String()
}

// ticketAddr is the address iteration prompts name for ticket id in the
// fixture epic under scratchDir.
func ticketAddr(scratchDir, epicName, id string) string {
	return tickets.ProjectName(scratchDir) + ":" + epicName + "/" + id
}

// mergeLockDir is where an epic's merge lock lives under fakes.StateDir, which
// TestMain sets.
func mergeLockDir(scratchDir, epic string) (string, error) {
	return mergeLockDirFor(fakes.TestDeps(), scratchDir, epic)
}

func mustMergeLockDir(t *testing.T, scratchDir, epic string) string {
	t.Helper()
	dir, err := mergeLockDir(scratchDir, epic)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
