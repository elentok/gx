package server

import (
	"errors"
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

// routingServer is configured with headless and can also build pty;
// RunnerFor hands out one runner per (runner, project) pair.
func routingServer(adjust ...func(*harnesschoice.Config)) (*harnesschoice.Harness, *namedRunner) {
	built := map[[2]string]*namedRunner{}
	configured := &namedRunner{Runner: runnerfake.NewRunner(), name: "headless", project: "_default"}
	cfg := harnesschoice.Config{
		Runner: configured,
		RunnerFor: func(runner, project string) agentrunner.Runner {
			if runner != "headless" && runner != "pty" {
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
		{"headless", "headless"},
		{"pty", "pty"},
	} {
		r, ok := s.RunnerForRun(runs.Run{Address: "proj:epic-a/01", Runner: tc.saved}).(*namedRunner)
		if !ok || r.name != tc.want || r.project != "proj" {
			t.Errorf("runner %q: got %+v, want %s for proj", tc.saved, r, tc.want)
		}
	}
	if r := s.RunnerForRun(runs.Run{Address: "proj:epic-a/01", Runner: ""}); r != nil {
		t.Errorf("empty runner name: got %v, want nil", r)
	}
}

func TestRunnerForRun_UnbuildableRunnerIsNil(t *testing.T) {
	t.Parallel()
	s, _ := routingServer()
	for _, name := range []string{"tmux", "herdr", ""} {
		if r := s.RunnerForRun(runs.Run{Address: "proj:epic-a/01", Runner: name}); r != nil {
			t.Errorf("runner %q: got %v, want nil", name, r)
		}
	}
}

func TestRunnerOf_UnbuildableRunnerIsAGoneAgent(t *testing.T) {
	t.Parallel()
	s, _ := routingServer()
	for _, name := range []string{"tmux", ""} {
		_, err := s.RunnerOf(runs.Run{Address: "proj:epic-a/01", Runner: name})
		if !errors.Is(err, agentrunner.ErrNotFound) {
			t.Errorf("runner %q: err = %v, want ErrNotFound", name, err)
		}
	}
}

func TestLiveSession_UnbuildableRunnerHasNoLiveSession(t *testing.T) {
	t.Parallel()
	reg, _, err := runs.Open(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	reg.PutRunIn("proj:epic-a/01", "proj:epic-a", t.TempDir(), "tmux", false)
	built := runnerfake.NewRunner()
	h := harnesschoice.New(harnesschoice.Deps{Host: fakeHarnessHost{}, Registry: reg, Config: harnesschoice.Config{
		Runner:    &namedRunner{Runner: built, name: "headless"},
		RunnerFor: func(string, string) agentrunner.Runner { return nil },
	}})
	addr := tickets.Address{Project: "proj", Epic: "epic-a", ID: "01"}
	runner, _, ok, err := h.LiveSession(addr)
	if runner != nil || ok || err != nil {
		t.Errorf("LiveSession = %v, %v, %v; want no runner, not ok, no error", runner, ok, err)
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
	if r := s.RunnerForRun(runs.Run{Address: "proj:epic-a/01", Runner: ""}); r != nil {
		t.Errorf("run with no runner name: want nil, got %v", r)
	}
}
