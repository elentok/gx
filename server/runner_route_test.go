package server

import (
	"testing"

	"github.com/elentok/gx/agentrunner"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets"
)

type namedRunner struct {
	agentrunner.Runner
	name, project string
}

func (r *namedRunner) Name() string { return r.name }

// fakeHarnessHost is a harnesschoice.Host with no sessions and no tickets on disk.
type fakeHarnessHost struct{}

func (fakeHarnessHost) ListSessions() ([]wire.SessionInfo, error) { return nil, nil }
func (fakeHarnessHost) FreshTicket(tickets.Address) (tickets.Epic, tickets.Ticket, bool) {
	return tickets.Epic{}, tickets.Ticket{}, false
}

// routingServer is configured with headless and can also build herdr and pty;
// RunnerFor hands out one runner per (runner, project) pair.
func routingServer(adjust ...func(*harnesschoice.Config)) (*harnesschoice.Harness, *namedRunner) {
	built := map[[2]string]*namedRunner{}
	configured := &namedRunner{Runner: runnerfake.NewRunner(), name: "headless", project: "_default"}
	cfg := harnesschoice.Config{
		Runner: configured,
		RunnerFor: func(runner, project string) agentrunner.Runner {
			if runner != "herdr" && runner != "headless" && runner != "pty" {
				return nil
			}
			key := [2]string{runner, project}
			if built[key] == nil {
				built[key] = &namedRunner{Runner: runnerfake.NewRunner(), name: runner, project: project}
			}
			return built[key]
		},
	}
	for _, f := range adjust {
		f(&cfg)
	}
	return harnesschoice.New(harnesschoice.Deps{Host: fakeHarnessHost{}, Config: cfg}), configured
}

func TestRunnerForRun_PicksTheRunsOwnRunner(t *testing.T) {
	t.Parallel()
	s, _ := routingServer()
	for _, tc := range []struct{ saved, want string }{
		{"herdr", "herdr"},
		{"headless", "headless"},
		{"pty", "pty"},
		{"", "herdr"},
	} {
		r, ok := s.RunnerForRun(runs.Run{Address: "proj:epic-a/01", Runner: tc.saved}).(*namedRunner)
		if !ok || r.name != tc.want || r.project != "proj" {
			t.Errorf("runner %q: got %+v, want %s for proj", tc.saved, r, tc.want)
		}
	}
}

func TestRunnerForRun_UnbuildableRunnerIsNil(t *testing.T) {
	t.Parallel()
	s, _ := routingServer()
	if r := s.RunnerForRun(runs.Run{Address: "proj:epic-a/01", Runner: "tmux"}); r != nil {
		t.Errorf("got %v, want nil", r)
	}
}

func TestRunnerFor_LaunchesStillUseTheConfiguredRunner(t *testing.T) {
	t.Parallel()
	s, _ := routingServer()
	r, ok := s.RunnerFor("proj").(*namedRunner)
	if !ok || r.name != "headless" || r.project != "proj" {
		t.Errorf("got %+v, want headless for proj", r)
	}
	if s.RunnerFor("proj") != agentrunner.Runner(r) {
		t.Error("a pair must keep one runner")
	}
}

func TestRunnerForRun_FallsBackToTheConfiguredRunnerByName(t *testing.T) {
	t.Parallel()
	s, configured := routingServer(func(c *harnesschoice.Config) { c.RunnerFor = nil })
	if r := s.RunnerForRun(runs.Run{Address: "proj:epic-a/01", Runner: "headless"}); r != agentrunner.Runner(configured) {
		t.Errorf("got %v, want the configured runner", r)
	}
	if r := s.RunnerForRun(runs.Run{Address: "proj:epic-a/01"}); r != nil {
		t.Errorf("herdr run under headless with no RunnerFor: got %v, want nil", r)
	}
}
