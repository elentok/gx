package catalog

import (
	"os"
	"path/filepath"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

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
