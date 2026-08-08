package git_test

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestTrackedFilesUnder(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.Mkdir(t, filepath.Join(dir, ".scratch", "an-epic"))
	testutil.WriteFile(t, dir, ".scratch/an-epic/leaked.txt", "tracked by mistake")
	testutil.CommitAll(t, dir, "commit .scratch")

	tracked, err := git.TrackedFilesUnder(dir, ".scratch")
	if err != nil {
		t.Fatalf("TrackedFilesUnder: %v", err)
	}
	if len(tracked) != 1 || tracked[0] != ".scratch/an-epic/leaked.txt" {
		t.Errorf("tracked = %v, want [.scratch/an-epic/leaked.txt]", tracked)
	}
}

func TestTrackedFilesUnder_NoneTracked(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.Mkdir(t, filepath.Join(dir, ".scratch", "an-epic"))
	testutil.WriteFile(t, dir, ".scratch/an-epic/untracked.txt", "not committed")

	tracked, err := git.TrackedFilesUnder(dir, ".scratch")
	if err != nil {
		t.Fatalf("TrackedFilesUnder: %v", err)
	}
	if len(tracked) != 0 {
		t.Errorf("tracked = %v, want none", tracked)
	}
}
