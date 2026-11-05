package ralphloop

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/ralphloop/internal/fakes"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/tickets"
	"go.uber.org/goleak"
)

// TestMain re-enters the test binary as the fake herdr helper when launched
// under herdrfake's coordinator, and gives merge locks a state dir.
// goleak.VerifyTestMain fails the specific leaking test at leak time instead
// of letting a stuck goroutine surface as a multi-minute package hang.
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

// setProcessEnv points the real process env var key at value for the rest of
// the test, restored on cleanup via a manual os.Setenv/os.Unsetenv pair
// rather than t.Setenv (which panics once a test has called t.Parallel()).
// Only for tests driving herdrfake helpers (RegisterCodexRollout,
// NewClaudeCompact) that read real process env directly and have no
// DepsOverrides-style seam of their own — everything else should use
// fakes.TestDepsWithOverrides instead. A test using this must stay non-parallel
// with any other test that also touches the same env var.
func setProcessEnv(t *testing.T, key, value string) {
	t.Helper()
	prev, had := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("Setenv %s: %v", key, err)
	}
	t.Cleanup(func() {
		if had {
			os.Setenv(key, prev)
		} else {
			os.Unsetenv(key)
		}
	})
}
