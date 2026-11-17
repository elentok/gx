package cmdtest

import (
	"os"
	"testing"

	"github.com/elentok/gx/server/servertest"
)

// Main is the body of a package's TestMain: it serves the session processes
// processes, then runs the tests. Tests get their
// dirs from EnvFor, so nothing here touches the process env.
func Main(m *testing.M) {
	servertest.RunSessionProcesses()
	os.Exit(m.Run())
}

// MainWithGx is Main plus the shared gx binary, built before any test starts
// so no test's time (or the slow-test limit) pays for the cold compile. A build
// error is kept and reported by SharedGx.
func MainWithGx(m *testing.M) {
	servertest.RunSessionProcesses()
	_, _ = buildSharedGx()
	code := m.Run()
	if sharedGxDir != "" {
		_ = os.RemoveAll(sharedGxDir)
	}
	os.Exit(code)
}
