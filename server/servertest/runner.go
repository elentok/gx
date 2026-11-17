package servertest

import (
	"sync"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
)

// runnerChoice is what the runner options recorded; startWith resolves it once
// every option has run, so the order of the options never matters.
type runnerChoice struct {
	runner    agentrunner.Runner
	runnerFor func(name, dir string) agentrunner.Runner
}

// choices maps a config under construction to its runnerChoice. Options are
// plain func(*server.Config), so this is how they reach the harness.
var choices sync.Map

func choiceOf(c *server.Config) *runnerChoice {
	v, _ := choices.Load(c)
	return v.(*runnerChoice)
}

// WithRunner makes r the configured runner and the runner for every pair.
func WithRunner(r agentrunner.Runner) func(*server.Config) {
	return func(c *server.Config) {
		ch := choiceOf(c)
		ch.runner = r
		ch.runnerFor = func(string, string) agentrunner.Runner { return r }
	}
}

// WithRunnerFor makes configured the configured runner and fn the runner for
// each pair.
func WithRunnerFor(configured agentrunner.Runner, fn func(name, dir string) agentrunner.Runner) func(*server.Config) {
	return func(c *server.Config) {
		ch := choiceOf(c)
		ch.runner = configured
		ch.runnerFor = fn
	}
}

// resolveRunner applies the recorded choice: WithRunner / WithRunnerFor first,
// then the PTY default. It reports whether the harness builds the PTY runner
// itself.
func resolveRunner(cfg *server.Config, ch *runnerChoice) (pty bool) {
	if ch.runner != nil {
		cfg.Runner = ch.runner
		cfg.RunnerFor = ch.runnerFor
		return false
	}
	return true
}
