package main

import (
	"os"
	"path/filepath"

	"github.com/elentok/gx/git/pr"
	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/testutil/ghfake"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

func (f *fixture) ghDir() string { return filepath.Join(f.root, "gh") }

// ghState is the fake gh's state file, which a flow edits to change CI.
func (f *fixture) ghState() string { return filepath.Join(f.ghDir(), "state.json") }

// origin is the bare repo the project repo's `origin` names; the path ends in
// the owner/repo the fake gh answers for.
func (f *fixture) origin() string {
	return filepath.Join(f.root, "origin", filepath.FromSlash(ghfake.Repo)+".git")
}

// setupFakeGH writes the fake gh's starting state and points the server at its
// directory. The gh binary itself is the one installBinaries put in the bin dir.
func (f *fixture) setupFakeGH(sc *spec.FakeGH) error {
	if err := os.MkdirAll(f.ghDir(), 0o755); err != nil {
		return err
	}
	f.env = append(f.env, ghfake.DirEnv+"="+f.ghDir())
	if err := ghfake.Locked(f.ghDir(), func(s *ghfake.State) error { *s = sc.State; return nil }); err != nil {
		return err
	}
	if err := os.MkdirAll(f.origin(), 0o755); err != nil {
		return err
	}
	return f.git(f.origin(), "init", "--bare", "-q", "-b", "main")
}

// addOrigin makes the bare origin the project repo's remote, with trunk pushed.
func (f *fixture) addOrigin(repo string) error {
	for _, args := range [][]string{{"remote", "add", "origin", f.origin()}, {"push", "-q", "origin", "main"}} {
		if err := f.git(repo, args...); err != nil {
			return err
		}
	}
	return nil
}

// forkFix forks the epic's ci-fix ticket for the scenario's failed check, as the
// watcher would, which also logs the "CI failed" entry.
func (f *fixture) forkFix(sc scenario) error {
	e := sc.Epics[0]
	_, _, err := prflow.ForkCIFix(filepath.Join(f.store, e.Project, e.Name), prflow.CIFixJob{
		Project: e.Project, Epic: e.Name, Number: spec.PRFlowNumber, URL: spec.PRFlowURL,
		HeadSHA: sc.FakeGH.State.PRs[0].HeadSHA, Check: pr.Check{Name: sc.FakeGH.FixCheck}, PerCheck: 3, PerPR: 10,
	})
	return err
}
