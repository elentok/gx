package main

import (
	"os"
	"path/filepath"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

// fakeNpmBody is the `npm ci` of the launching scenario: it marks that it
// started and waits while the npm-hold file exists, so the ticket stays at the
// installing-dependencies launch step until a test removes the hold.
const fakeNpmBody = `#!/bin/sh
dir=$(dirname "$0")/..
touch "$dir/npm-started"
while [ -e "$dir/npm-hold" ]; do sleep 0.05; done
`

// failingNpmBody is the `npm ci` of the launch-fail scenario: it exits at once
// with a message the pause reason carries.
const failingNpmBody = `#!/bin/sh
echo "npm ci: registry unreachable" >&2
exit 1
`

// addLockfile commits a tracked npm lockfile, so a launch installs dependencies.
// gitSetup runs after the repo's first commit, and the worktrees branch from it.
func addLockfile(git spec.Git, repo string) error {
	for name, body := range map[string]string{"package.json": "{}\n", "package-lock.json": "{}\n"} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0o644); err != nil {
			return err
		}
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "add lockfile"}} {
		if err := git(repo, args...); err != nil {
			return err
		}
	}
	return nil
}

var (
	// launchHeld queues one ticket whose worktree has a tracked npm lockfile, so
	// its launch stops at "installing dependencies" until <gate dir>/npm-hold is
	// removed; then it moves on to starting the agent. npm-started marks the wait.
	launchHeld = scenario{
		Epics:    []epic{{Project: project, Name: "launch-epic", Tickets: []ticket{{ID: "01", Slug: "launching", Opts: servertest.TicketOpts{}}}}},
		Queue:    []string{project + ":launch-epic"},
		GitSetup: addLockfile,
		FakeNpm:  true,
	}
	// launchFail is launchHeld with an npm that fails, so the launch fails and
	// the ticket pauses as iteration-error.
	launchFail = scenario{
		Epics:      []epic{{Project: project, Name: "launch-epic", Tickets: []ticket{{ID: "01", Slug: "launching", Opts: servertest.TicketOpts{}}}}},
		Queue:      []string{project + ":launch-epic"},
		GitSetup:   addLockfile,
		FakeNpm:    true,
		FailingNpm: true,
	}
	// epicAtCap queues an epic of four independent tickets with held workers
	// under a per-epic cap of 2, so the later tickets read "concurrency
	// cap". The cap is in the config file at ConfigFile: a test lowers it there
	// and runs `gx server reload-config` to get the over-cap case.
	epicAtCap = scenario{
		Epics: []epic{{Project: project, Name: "cap-epic", Tickets: []ticket{
			{ID: "01", Slug: "first", Opts: servertest.TicketOpts{}},
			{ID: "02", Slug: "second", Opts: servertest.TicketOpts{}},
			{ID: "03", Slug: "third", Opts: servertest.TicketOpts{}},
			{ID: "04", Slug: "fourth", Opts: servertest.TicketOpts{}},
		}}},
		Queue:  []string{project + ":cap-epic"},
		Hold:   true,
		Config: []string{"execution-queue.max-agents-per-epic=2"},
	}
)

// setupFakeNpm writes <gate dir>/bin/npm and starts the hold on it.
func (f *fixture) setupFakeNpm(failing bool) error {
	body := fakeNpmBody
	if failing {
		body = failingNpmBody
	}
	if err := f.putBinFirst(map[string]string{"npm": body}); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(f.gateDir(), "npm-hold"), nil, 0o644)
}

// configFile is the global config the server loaded; a test edits it and reloads.
func (f *fixture) configFile() string {
	return filepath.Join(f.root, "home", ".config", "gx", "config.json")
}
