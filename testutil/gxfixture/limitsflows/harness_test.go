package limitsflows

import (
	"os"
	"testing"

	"github.com/elentok/gx/testutil/gxfixture/fixturetest"
)

// running is a started fixture; the scenario helpers in this package's test
// files are methods on it.
type running struct{ *fixturetest.Running }

// web is a client of the fixture's web server.
type web = fixturetest.Web

func start(t *testing.T, scenario string) *running {
	t.Helper()
	return &running{fixturetest.Start(t, scenario)}
}

func startWith(t *testing.T, scenario string, env []string, flags ...string) *running {
	t.Helper()
	return &running{fixturetest.StartWith(t, scenario, env, flags...)}
}

// launch is startWith with the kill cleanup handed to register, for a fixture
// that outlives one test.
func launch(t *testing.T, register func(func()), scenario string, env []string, flags ...string) *running {
	t.Helper()
	return &running{fixturetest.Launch(t, register, scenario, env, flags...)}
}

func TestMain(m *testing.M) {
	os.Exit(fixturetest.Run(m, nil))
}
