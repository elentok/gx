package server

import (
	"fmt"
	"github.com/elentok/gx/server/runs"
	"log/slog"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// runnerFor is the configured runner for project: the one new launches use.
func (s *Server) runnerFor(project string) agentrunner.Runner {
	return s.runnerNamed(s.cfg.Runner.Name(), project)
}

// launchRunner is the runner a new launch of agent in project uses. The
// headless runner hosts only Claude, so Codex runs on pty unless herdr is
// configured.
func (s *Server) launchRunner(project string, agent ralphloop.AgentKind) agentrunner.Runner {
	if agent == ralphloop.AgentCodex && s.cfg.Runner.Name() != runs.RunnerHerdr {
		return s.runnerNamed(string(agentrunner.ChoicePTY), project)
	}
	return s.runnerFor(project)
}

// RunnerForRun is the runner that started run, whatever the configured one is.
// It is nil when that runner cannot be built.
func (s *Server) RunnerForRun(run runs.Run) agentrunner.Runner {
	name := run.Runner
	if name == "" {
		name = runs.RunnerHerdr
	}
	project := ""
	if addr, err := tickets.ParseAddress(run.Address, tickets.AddressContext{}); err == nil {
		project = addr.Project
	}
	return s.runnerNamed(name, project)
}

// runnerOf is runnerForRun, failing when the run's runner cannot be built.
func (s *Server) runnerOf(run runs.Run) (agentrunner.Runner, error) {
	if r := s.RunnerForRun(run); r != nil {
		return r, nil
	}
	name := run.Runner
	if name == "" {
		name = runs.RunnerHerdr
	}
	return nil, fmt.Errorf("started by %s; %s unavailable", name, name)
}

// liveSession is the session of address's live iteration and the runner
// hosting it: a tracked run's own runner, else the configured one (a paused
// ticket's agent can outlive its run).
func (s *Server) liveSession(addr tickets.Address) (agentrunner.Runner, agentrunner.Session, bool, error) {
	runner, err := s.iterationRunner(addr)
	if err != nil {
		return nil, agentrunner.Session{}, false, err
	}
	label, _, _ := ralphloop.IterationIdentity(addr.Epic, addr.ID, "")
	sess, ok, err := runner.Find(label)
	return runner, sess, ok, err
}

// iterationRunner is the runner hosting address's iteration: a tracked run's
// own runner, else the configured one.
func (s *Server) iterationRunner(addr tickets.Address) (agentrunner.Runner, error) {
	if run, ok := s.registry.RunOf(addr.String()); ok {
		return s.runnerOf(run)
	}
	agent := s.DefaultHarness(addr.Project)
	if _, t, ok := s.freshTicket(addr); ok {
		agent = s.resolveTicket(addr, t).Harness
	}
	return s.launchRunner(addr.Project, agent), nil
}

// giveLog hands the server logger to a runner that logs the vim probe's
// decisions. Runners are built before the logger exists, and a built runner is
// shared, so the first handover must finish before any caller uses it.
func (s *Server) giveLog(r agentrunner.Runner) {
	s.logGiven.Lock()
	defer s.logGiven.Unlock()
	if l, ok := r.(interface{ SetLog(*slog.Logger) }); ok {
		l.SetLog(s.log)
	}
}

func (s *Server) runnerNamed(name, project string) agentrunner.Runner {
	if s.cfg.RunnerFor != nil {
		if r := s.cfg.RunnerFor(name, project); r != nil {
			s.giveLog(r)
			return r
		}
	}
	if name == s.cfg.Runner.Name() {
		return s.cfg.Runner
	}
	return nil
}
