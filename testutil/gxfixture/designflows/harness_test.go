package designflows

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/testutil/gxfixture/fixturetest"
	"github.com/elentok/gx/testutil/gxfixture/limitseed"
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

const project = limitseed.Project

// postWhenIdle is post, retried while the server still reports the session as
// working: its status lags the fake's turn, as promptWhenIdle describes.
func (r *running) postWhenIdle(t *testing.T, id, path string, body any) {
	t.Helper()
	w := r.Web(t)
	deadline := time.Now().Add(20 * time.Second)
	for {
		data, ok := w.Try(t, path, body)
		if ok {
			return
		}
		if !strings.Contains(data, "session-not-idle") || time.Now().After(deadline) {
			t.Fatalf("POST %s refused: %s", path, data)
		}
		r.waitIdle(t, id)
	}
}

func TestMain(m *testing.M) {
	os.Exit(fixturetest.Run(m, func() (func(), error) {
		// The shared fixtures' seeding is slow, so they start here, outside
		// every test's time limit. A -run that skips their tests skips them.
		var kills []func()
		teardown := func() {
			for _, k := range kills {
				k()
			}
		}
		for _, f := range []struct {
			part, scenario string
			into           **running
		}{{"Blocked", "agents-blocked", &blocked}, {"Workspaces", "workspaces", &workspaces}} {
			if !fixturetest.RunMatches(f.part) {
				continue
			}
			r, err := fixturetest.LaunchProc(func(func()) {}, f.scenario, nil)
			if r != nil {
				*f.into = &running{r}
				kills = append(kills, r.Kill)
			}
			if err != nil {
				return teardown, fmt.Errorf("%s fixture: %v", f.scenario, err)
			}
		}
		return teardown, nil
	}))
}
