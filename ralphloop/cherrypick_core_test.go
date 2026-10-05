package ralphloop

import (
	"errors"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestCherryPickCore_CleanPick_NotConflicted(t *testing.T) {
	dir := testutil.TempRepo(t)
	base, _ := git.RevParse(dir, "HEAD")
	testutil.MustGitExported(t, dir, "checkout", "-b", "iter", base)
	testutil.WriteFile(t, dir, "a.txt", "a\n")
	testutil.CommitAll(t, dir, "Add a")
	tip, _ := git.RevParse(dir, "HEAD")
	testutil.MustGitExported(t, dir, "checkout", "main")

	conflicted, err := cherryPickCore(landDepsFor(testDeps()), LandParams{FeatureWorktree: dir, FeatureBranch: "main", SourceRange: SourceRange{Base: base, Tip: tip}})
	if err != nil || conflicted {
		t.Fatalf("cherryPickCore = (%v, %v), want (false, nil)", conflicted, err)
	}
}

func TestCherryPickCore_Conflict_LeavesSequencerInProgress(t *testing.T) {
	dir := testutil.TempRepo(t)
	base, _ := git.RevParse(dir, "HEAD")
	testutil.MustGitExported(t, dir, "checkout", "-b", "iter", base)
	testutil.WriteFile(t, dir, "shared.txt", "iteration\n")
	testutil.CommitAll(t, dir, "iter shared")
	tip, _ := git.RevParse(dir, "HEAD")
	testutil.MustGitExported(t, dir, "checkout", "main")
	testutil.WriteFile(t, dir, "shared.txt", "main\n")
	testutil.CommitAll(t, dir, "main shared")

	conflicted, err := cherryPickCore(landDepsFor(testDeps()), LandParams{FeatureWorktree: dir, FeatureBranch: "main", SourceRange: SourceRange{Base: base, Tip: tip}})
	if err != nil || !conflicted {
		t.Fatalf("cherryPickCore = (%v, %v), want (true, nil)", conflicted, err)
	}
	if inProgress, _ := git.CherryPickInProgress(dir); !inProgress {
		t.Error("sequencer not in progress after conflict, want it left intact")
	}
}

func TestCherryPickCore_NonConflictFailure_ReturnsError(t *testing.T) {
	d := testDeps()
	d.CherryPickRange = func(dir, from, to string) error { return errors.New("boom") }
	d.CherryPickInProgress = func(dir string) (bool, error) { return false, nil }

	conflicted, err := cherryPickCore(landDepsFor(d), LandParams{FeatureBranch: "main", SourceRange: SourceRange{Base: "b", Tip: "t"}})
	if err == nil || conflicted {
		t.Fatalf("cherryPickCore = (%v, %v), want (false, error)", conflicted, err)
	}
}
