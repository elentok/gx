package server

import (
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/testutil/runnerfake"
)

type namedRunner struct {
	agentrunner.Runner
	name, project string
}

func (r *namedRunner) Name() string { return r.name }

// routingServer is configured with headless and can also build herdr and pty;
// RunnerFor hands out one runner per (runner, project) pair.
func routingServer() (*Server, *namedRunner) {
	built := map[[2]string]*namedRunner{}
	configured := &namedRunner{Runner: runnerfake.NewRunner(), name: "headless", project: "_default"}
	return &Server{cfg: Config{
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
	}}, configured
}

func TestRunnerForRun_PicksTheRunsOwnRunner(t *testing.T) {
	s, _ := routingServer()
	for _, tc := range []struct{ saved, want string }{
		{"herdr", "herdr"},
		{"headless", "headless"},
		{"pty", "pty"},
		{"", "herdr"},
	} {
		r, ok := s.runnerForRun(Run{Address: "proj:epic-a/01", Runner: tc.saved}).(*namedRunner)
		if !ok || r.name != tc.want || r.project != "proj" {
			t.Errorf("runner %q: got %+v, want %s for proj", tc.saved, r, tc.want)
		}
	}
}

func TestRunnerForRun_UnbuildableRunnerIsNil(t *testing.T) {
	s, _ := routingServer()
	if r := s.runnerForRun(Run{Address: "proj:epic-a/01", Runner: "tmux"}); r != nil {
		t.Errorf("got %v, want nil", r)
	}
}

func TestRunnerFor_LaunchesStillUseTheConfiguredRunner(t *testing.T) {
	s, _ := routingServer()
	r, ok := s.runnerFor("proj").(*namedRunner)
	if !ok || r.name != "headless" || r.project != "proj" {
		t.Errorf("got %+v, want headless for proj", r)
	}
	if s.runnerFor("proj") != agentrunner.Runner(r) {
		t.Error("a pair must keep one runner")
	}
}

func TestRunnerForRun_FallsBackToTheConfiguredRunnerByName(t *testing.T) {
	s, configured := routingServer()
	s.cfg.RunnerFor = nil
	if r := s.runnerForRun(Run{Address: "proj:epic-a/01", Runner: "headless"}); r != agentrunner.Runner(configured) {
		t.Errorf("got %v, want the configured runner", r)
	}
	if r := s.runnerForRun(Run{Address: "proj:epic-a/01"}); r != nil {
		t.Errorf("herdr run under headless with no RunnerFor: got %v, want nil", r)
	}
}
