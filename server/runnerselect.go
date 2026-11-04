package server

import (
	"fmt"
	"log/slog"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/tickets"
)

// RunnerFor is the configured runner for project: the one new launches use.
func (h *Harness) RunnerFor(project string) agentrunner.Runner {
	return h.RunnerNamed(h.cfg.Runner.Name(), project)
}

// LaunchRunner is the runner a new launch of agent in project uses. The
// headless runner hosts only Claude, so Codex runs on pty unless herdr is
// configured.
func (h *Harness) LaunchRunner(project string, agent ralphloop.AgentKind) agentrunner.Runner {
	if agent == ralphloop.AgentCodex && h.cfg.Runner.Name() != runs.RunnerHerdr {
		return h.RunnerNamed(string(agentrunner.ChoicePTY), project)
	}
	return h.RunnerFor(project)
}

// RunnerForRun is the runner that started run, whatever the configured one is.
// It is nil when that runner cannot be built.
func (h *Harness) RunnerForRun(run runs.Run) agentrunner.Runner {
	name := run.Runner
	if name == "" {
		name = runs.RunnerHerdr
	}
	project := ""
	if addr, err := tickets.ParseAddress(run.Address, tickets.AddressContext{}); err == nil {
		project = addr.Project
	}
	return h.RunnerNamed(name, project)
}

// RunnerOf is RunnerForRun, failing when the run's runner cannot be built.
func (h *Harness) RunnerOf(run runs.Run) (agentrunner.Runner, error) {
	if r := h.RunnerForRun(run); r != nil {
		return r, nil
	}
	name := run.Runner
	if name == "" {
		name = runs.RunnerHerdr
	}
	return nil, fmt.Errorf("started by %s; %s unavailable", name, name)
}

// LiveSession is the session of address's live iteration and the runner
// hosting it: a tracked run's own runner, else the configured one (a paused
// ticket's agent can outlive its run).
func (h *Harness) LiveSession(addr tickets.Address) (agentrunner.Runner, agentrunner.Session, bool, error) {
	runner, err := h.IterationRunner(addr)
	if err != nil {
		return nil, agentrunner.Session{}, false, err
	}
	label, _, _ := ralphloop.IterationIdentity(addr.Epic, addr.ID, "")
	sess, ok, err := runner.Find(label)
	return runner, sess, ok, err
}

// IterationRunner is the runner hosting address's iteration: a tracked run's
// own runner, else the configured one.
func (h *Harness) IterationRunner(addr tickets.Address) (agentrunner.Runner, error) {
	if run, ok := h.registry.RunOf(addr.String()); ok {
		return h.RunnerOf(run)
	}
	agent := h.DefaultHarness(addr.Project)
	if _, t, ok := h.host.FreshTicket(addr); ok {
		agent = h.ResolveTicket(addr, t).Harness
	}
	return h.LaunchRunner(addr.Project, agent), nil
}

// GiveLog hands the server logger to a runner that logs the vim probe's
// decisions. Runners are built before the logger exists, and a built runner is
// shared, so the first handover must finish before any caller uses it.
func (h *Harness) GiveLog(r agentrunner.Runner) {
	h.logGiven.Lock()
	defer h.logGiven.Unlock()
	if l, ok := r.(interface{ SetLog(*slog.Logger) }); ok {
		l.SetLog(h.log)
	}
}

// RunnerNamed is the runner called name for project, nil when it cannot be built.
func (h *Harness) RunnerNamed(name, project string) agentrunner.Runner {
	if h.cfg.RunnerFor != nil {
		if r := h.cfg.RunnerFor(name, project); r != nil {
			h.GiveLog(r)
			return r
		}
	}
	if name == h.cfg.Runner.Name() {
		return h.cfg.Runner
	}
	return nil
}
