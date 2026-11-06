package ralphloop

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/tickets"
)

// installRecorder is a fake InstallDeps that records the directories it was
// asked about and fails when err is set.
type installRecorder struct {
	paths []string
	err   error
}

func (r *installRecorder) install(_ context.Context, path, _ string) ([]string, error) {
	r.paths = append(r.paths, path)
	return []string{"fake install"}, r.err
}

func prepareDepsFixture(t *testing.T) (deps.Deps, *installRecorder, OneIteration) {
	t.Helper()
	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	gitIn(t, repo, "commit", "--allow-empty", "-m", "root")
	rec := &installRecorder{}
	d := deps.DefaultDeps()
	d.InstallDeps = rec.install
	wtDir := filepath.Join(t.TempDir(), "wt")
	d.WorktreeDir = func(string) (string, error) { return wtDir, nil }
	scratchDir := fixture.WriteEpic(t, "fix-it", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	o := OneIteration{
		RepoDir: repo, Epic: "fix-it", RootBase: "main", ScratchDir: scratchDir,
		Ticket: tickets.Ticket{Identifier: "01"},
	}
	return d, rec, o
}

func TestPrepareIteration_InstallsOnlyInTheIterationWorktree(t *testing.T) {
	d, rec, o := prepareDepsFixture(t)

	w, err := PrepareIteration(d, o)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{w.Path}; !slices.Equal(rec.paths, want) {
		t.Errorf("installed in %v, want %v (not the feature worktree %s)", rec.paths, want, w.featureWorktree)
	}
}

func TestPrepareFeature_InstallsInTheFeatureWorktreeOnEveryPrepare(t *testing.T) {
	d, rec, o := prepareDepsFixture(t)

	w, err := PrepareFeature(d, o)
	if err != nil {
		t.Fatal(err)
	}
	// The worktree already exists now; a real second add would be rejected.
	d.AddWorktree = func(string, string, string, string) error { return nil }
	if _, err := PrepareFeature(d, o); err != nil {
		t.Fatal(err)
	}
	if want := []string{w.Path, w.Path}; !slices.Equal(rec.paths, want) {
		t.Errorf("installed in %v, want %v", rec.paths, want)
	}
}

func TestPrepareCommitless_InstallsInADetachedWorktreeButNotAScratchDir(t *testing.T) {
	d, rec, o := prepareDepsFixture(t)

	if _, err := PrepareCommitless(d, o, filepath.Join(t.TempDir(), "scratch"), ""); err != nil {
		t.Fatal(err)
	}
	if len(rec.paths) != 0 {
		t.Fatalf("scratch dir prepare installed in %v", rec.paths)
	}
	w, err := PrepareCommitless(d, o, "", "main")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{w.Path}; !slices.Equal(rec.paths, want) {
		t.Errorf("installed in %v, want %v", rec.paths, want)
	}
}

func TestResume_NeverInstalls(t *testing.T) {
	d, rec, o := prepareDepsFixture(t)

	if _, err := ResumeIteration(d, o, "base"); err != nil {
		t.Fatal(err)
	}
	if _, err := ResumeFeature(d, o); err != nil {
		t.Fatal(err)
	}
	for _, scratch := range []string{"", t.TempDir()} {
		if _, err := ResumeCommitless(d, o, scratch); err != nil {
			t.Fatal(err)
		}
	}
	if len(rec.paths) != 0 {
		t.Errorf("a resume installed in %v", rec.paths)
	}
}

func TestPrepare_ReturnsTheInstallErrorUnwrapped(t *testing.T) {
	prepares := map[string]func(deps.Deps, OneIteration) error{
		"iteration": func(d deps.Deps, o OneIteration) error { _, err := PrepareIteration(d, o); return err },
		"feature":   func(d deps.Deps, o OneIteration) error { _, err := PrepareFeature(d, o); return err },
		"commitless": func(d deps.Deps, o OneIteration) error {
			_, err := PrepareCommitless(d, o, "", "main")
			return err
		},
	}
	for name, prepare := range prepares {
		t.Run(name, func(t *testing.T) {
			d, rec, o := prepareDepsFixture(t)
			rec.err = errors.New("installing dependencies in x: boom")

			if err := prepare(d, o); err != rec.err {
				t.Errorf("err = %v, want the install error itself", err)
			}
		})
	}
}

func TestPrepareIteration_LogsDepsInstalled(t *testing.T) {
	d, _, o := prepareDepsFixture(t)

	if _, err := PrepareIteration(d, o); err != nil {
		t.Fatal(err)
	}
	evs, _, err := runlog.ReadEvents(o.ScratchDir, o.Epic)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Type != "deps-installed" || evs[0].Ticket != "01" {
		t.Errorf("events = %+v, want one deps-installed for ticket 01", evs)
	}
}

func TestPrepare_RebasesAnExistingEpicBranchOntoTheBaseOnFirstAttach(t *testing.T) {
	for name, prepare := range map[string]func(deps.Deps, OneIteration) error{
		"iteration": func(d deps.Deps, o OneIteration) error { _, err := PrepareIteration(d, o); return err },
		"feature":   func(d deps.Deps, o OneIteration) error { _, err := PrepareFeature(d, o); return err },
	} {
		t.Run(name, func(t *testing.T) {
			d, _, o := prepareDepsFixture(t)
			gitIn(t, o.RepoDir, "branch", o.Epic)
			gitIn(t, o.RepoDir, "commit", "--allow-empty", "-m", "newer trunk commit")

			if err := prepare(d, o); err != nil {
				t.Fatal(err)
			}
			if ok, err := d.IsAncestor(o.RepoDir, "main", o.Epic); err != nil || !ok {
				t.Errorf("main is not an ancestor of %s after the first attach (err %v)", o.Epic, err)
			}
		})
	}
}

func TestPrepareFeature_FailsWhenTheRebaseConflicts(t *testing.T) {
	d, _, o := prepareDepsFixture(t)
	gitIn(t, o.RepoDir, "branch", o.Epic)
	gitIn(t, o.RepoDir, "commit", "--allow-empty", "-m", "newer trunk commit")
	d.RebaseOntoOrAbort = func(string, string) error { return errors.New("conflict") }

	_, err := PrepareFeature(d, o)
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("PrepareFeature err = %v, want the rebase conflict", err)
	}
}

func TestPrepareFeature_RebasesWhenTheWorktreeAlreadyExists(t *testing.T) {
	d, _, o := prepareDepsFixture(t)
	gitIn(t, o.RepoDir, "branch", o.Epic)
	if _, err := PrepareFeature(d, o); err != nil {
		t.Fatal(err)
	}
	gitIn(t, o.RepoDir, "commit", "--allow-empty", "-m", "newer trunk commit")

	if _, err := PrepareFeature(d, o); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.IsAncestor(o.RepoDir, "main", o.Epic); err != nil || !ok {
		t.Errorf("main is not an ancestor of %s after a second attach (err %v)", o.Epic, err)
	}
}
