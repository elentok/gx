package ralphloop

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/tickets"
)

// installRecorder is a fake InstallDeps that records the directories it was
// asked about and fails when err is set.
type installRecorder struct {
	paths []string
	err   error
}

func (r *installRecorder) install(path, _ string) ([]string, error) {
	r.paths = append(r.paths, path)
	return []string{"fake install"}, r.err
}

func prepareDepsFixture(t *testing.T) (Deps, *installRecorder, OneIteration) {
	t.Helper()
	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	gitIn(t, repo, "commit", "--allow-empty", "-m", "root")
	rec := &installRecorder{}
	d := DefaultDeps()
	d.InstallDeps = rec.install
	wtDir := filepath.Join(t.TempDir(), "wt")
	d.WorktreeDir = func(string) (string, error) { return wtDir, nil }
	scratchDir := writeEpic(t, "fix-it", map[string]string{
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
	prepares := map[string]func(Deps, OneIteration) error{
		"iteration": func(d Deps, o OneIteration) error { _, err := PrepareIteration(d, o); return err },
		"feature":   func(d Deps, o OneIteration) error { _, err := PrepareFeature(d, o); return err },
		"commitless": func(d Deps, o OneIteration) error {
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
	evs, _, err := ReadEvents(o.ScratchDir, o.Epic)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Type != "deps-installed" || evs[0].Ticket != "01" {
		t.Errorf("events = %+v, want one deps-installed for ticket 01", evs)
	}
}
