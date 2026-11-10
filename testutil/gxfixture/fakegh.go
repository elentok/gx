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
func (f *fixture) addOrigin(repo string, sc *spec.FakeGH) error {
	for _, args := range [][]string{{"remote", "add", "origin", f.origin()}, {"push", "-q", "origin", "main"}} {
		if err := f.git(repo, args...); err != nil {
			return err
		}
	}
	if sc.MergedFile != "" {
		return f.mergeOnOrigin(repo, sc.MergedFile)
	}
	return nil
}

// mergeOnOrigin puts a commit adding file on origin's main, which local Trunk
// does not have, and leaves an uncommitted file of that name in Trunk.
func (f *fixture) mergeOnOrigin(repo, file string) error {
	if err := os.WriteFile(filepath.Join(repo, file), []byte("merged"), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"add", file}, {"commit", "-m", "merge PR"}, {"push", "-q", "origin", "main"}, {"reset", "-q", "--hard", "HEAD~1"}} {
		if err := f.git(repo, args...); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(repo, file), []byte("local"), 0o644)
}

// seedPRFlow writes what the scenario's epic holds beyond tickets and frontmatter:
// ticket Resolutions, the activity log, and the forked ci-fix ticket.
func (f *fixture) seedPRFlow(sc scenario) error {
	e := sc.Epics[0]
	epicDir := filepath.Join(f.store, e.Project, e.Name)
	for _, tk := range e.Tickets {
		res, ok := sc.FakeGH.Resolutions[tk.ID]
		if !ok {
			continue
		}
		path := filepath.Join(epicDir, "issues", tk.ID+"-"+tk.Slug+".md")
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, append(body, []byte("\n## Resolution\n\n"+res+"\n")...), 0o644); err != nil {
			return err
		}
	}
	for _, en := range sc.FakeGH.Activity {
		if err := prflow.Append(epicDir, en); err != nil {
			return err
		}
	}
	if sc.FakeGH.FixCheck != "" {
		return f.forkFix(sc)
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
