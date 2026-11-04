package servertest

import (
	"sync"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/runnerbuild"
	"github.com/elentok/gx/server"
)

// HerdrReason says why a test turns the fake herdr on.
type HerdrReason int

const (
	// HerdrIsTheSubject is for a test of herdr itself: the herdr runner, its
	// failures, or herdr-only behaviour.
	HerdrIsTheSubject HerdrReason = iota + 1
)

// runnerChoice is what the runner options recorded; startWith resolves it once
// every option has run, so the order of the options never matters.
type runnerChoice struct {
	runner    agentrunner.Runner
	runnerFor func(name, dir string) agentrunner.Runner
	herdr     bool
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

// WithHerdr turns the fake herdr on. It makes herdr the configured runner only
// when no WithRunner or WithRunnerFor is given.
func WithHerdr(HerdrReason) func(*server.Config) {
	return func(c *server.Config) { choiceOf(c).herdr = true }
}

// resolveRunner applies the recorded choice: WithRunner / WithRunnerFor first,
// then the herdr runner built here for WithHerdr. It reports whether the harness
// builds the PTY runner itself, the default.
func resolveRunner(cfg *server.Config, ch *runnerChoice) (pty bool) {
	if ch.runner != nil {
		cfg.Runner = ch.runner
		cfg.RunnerFor = ch.runnerFor
		return false
	}
	if ch.herdr {
		cfg.Runner, _ = runnerbuild.New(agentrunner.ChoiceHerdr, cfg.Env, cfg.StateDir, cfg.RunnerWait, nil)
	}
	return !ch.herdr
}
