package main

import (
	"os"
	"path/filepath"

	"github.com/elentok/gx/server/servertest"
)

// fakeNpmBody is the `npm ci` of the launching scenario: it marks that it
// started and waits while the npm-hold file exists, so the ticket stays at the
// installing-dependencies launch step until a test removes the hold.
const fakeNpmBody = `#!/bin/sh
dir=$(dirname "$0")/..
touch "$dir/npm-started"
while [ -e "$dir/npm-hold" ]; do sleep 0.05; done
`

var (
	// launchHeld queues one ticket whose worktree has a tracked npm lockfile, so
	// its launch stops at "installing dependencies" until <gate dir>/npm-hold is
	// removed; then it moves on to starting the agent. npm-started marks the wait.
	launchHeld = scenario{
		epics: []epic{{project, "launch-epic", []ticket{{"01", "launching", servertest.TicketOpts{}}}}},
		queue: []string{project + ":launch-epic"},
		// gitSetup runs after the repo's first commit, and the worktrees branch from it.
		gitSetup: func(f *fixture, repo string) error {
			for name, body := range map[string]string{"package.json": "{}\n", "package-lock.json": "{}\n"} {
				if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0o644); err != nil {
					return err
				}
			}
			for _, args := range [][]string{{"add", "."}, {"commit", "-m", "add lockfile"}} {
				if err := f.git(repo, args...); err != nil {
					return err
				}
			}
			return nil
		},
		fakeNpm: true,
	}
	// epicAtCap queues an epic of four independent tickets with held workers
	// under a per-epic cap of 2, so the later tickets read "concurrency
	// cap". The cap is in the config file at ConfigFile: a test lowers it there
	// and runs `gx server reload-config` to get the over-cap case.
	epicAtCap = scenario{
		epics: []epic{{project, "cap-epic", []ticket{
			{"01", "first", servertest.TicketOpts{}},
			{"02", "second", servertest.TicketOpts{}},
			{"03", "third", servertest.TicketOpts{}},
			{"04", "fourth", servertest.TicketOpts{}},
		}}},
		queue:  []string{project + ":cap-epic"},
		hold:   true,
		config: []string{"execution-queue.max-agents-per-epic=2"},
	}
)

// setupFakeNpm writes <gate dir>/bin/npm and starts the hold on it.
func (f *fixture) setupFakeNpm() error {
	if err := f.putBinFirst(map[string]string{"npm": fakeNpmBody}); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(f.gateDir(), "npm-hold"), nil, 0o644)
}

// configFile is the global config the server loaded; a test edits it and reloads.
func (f *fixture) configFile() string {
	return filepath.Join(f.root, "home", ".config", "gx", "config.json")
}
