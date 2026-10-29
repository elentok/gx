// Package runnerbuild builds agent runners by name for a project. The server
// and the test harness share it, so a test drives the runners production does.
package runnerbuild

import (
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/herdrrunner"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/sessionlookup"
)

// FallbackProject is the project the default runner is built for.
const FallbackProject = "_default"

// PtyOverride replaces the Pty runner's holder, agent program, process table
// and timings. Production passes nil, which leaves every field to its default.
type PtyOverride struct {
	Holder       []string
	Claude       string
	Procs        nativerunner.ProcessTable
	PollInterval time.Duration
	StartTimeout time.Duration
	PromptGrace  time.Duration
	StopGrace    time.Duration
}

// New returns the runner for choice c, plus runnerFor, which builds any runner
// by name for a project. Each (runner, project) pair is built once and kept,
// because a native runner's live sessions are in memory and its agent dirs are
// per project; runnerFor is nil for a runner it cannot build. The returned
// runner answers host health and projects runnerFor never sees, so it is built
// for FallbackProject.
func New(c agentrunner.Choice, env []string, stateDir string, n time.Duration, pty *PtyOverride) (r agentrunner.Runner, runnerFor func(runner, project string) agentrunner.Runner) {
	type pair struct{ runner, project string }
	dirs := config.DirsFrom(env)
	home, _ := dirs.Home()
	var mu sync.Mutex
	built := map[pair]agentrunner.Runner{}
	runnerFor = func(runner, project string) agentrunner.Runner {
		mu.Lock()
		defer mu.Unlock()
		key := pair{runner, project}
		if r, ok := built[key]; ok {
			return r
		}
		var r agentrunner.Runner
		switch agentrunner.Choice(runner) {
		case agentrunner.ChoiceHerdr:
			h := herdrrunner.NewIn(herdr.CLI{Env: env}, home, sessionlookup.RootsFrom(env).CodexHome)
			h.N = n
			r = h
		case agentrunner.ChoiceHeadless:
			r = &nativerunner.Headless{Root: nativerunner.AgentsRoot(stateDir, project), N: n}
		case agentrunner.ChoicePTY:
			p := &nativerunner.Pty{Root: nativerunner.AgentsRoot(stateDir, project), N: n, Env: env}
			if pty != nil {
				p.Holder = pty.Holder
				p.Claude = pty.Claude
				p.Procs = pty.Procs
				p.PollInterval = pty.PollInterval
				p.StartTimeout = pty.StartTimeout
				p.PromptGrace = pty.PromptGrace
				p.StopGrace = pty.StopGrace
			}
			r = p
		default:
			return nil
		}
		built[key] = r
		return r
	}
	return runnerFor(string(c), FallbackProject), runnerFor
}
