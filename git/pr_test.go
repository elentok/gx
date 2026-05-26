package git_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestIsCommitMergedToMain_merged(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)

	hash := headHash(t, repoDir)

	merged, err := git.IsCommitMergedToMain(repoDir, hash)
	if err != nil {
		t.Fatalf("IsCommitMergedToMain: %v", err)
	}
	if !merged {
		t.Error("expected commit on main to be merged to main")
	}
}

func TestIsCommitMergedToMain_unmerged(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)

	testutil.MustGitExported(t, repoDir, "checkout", "-b", "feature")
	testutil.WriteFile(t, repoDir, "feature.txt", "feature")
	testutil.MustGitExported(t, repoDir, "add", ".")
	testutil.MustGitExported(t, repoDir, "commit", "-m", "feature commit")

	hash := headHash(t, repoDir)

	merged, err := git.IsCommitMergedToMain(repoDir, hash)
	if err != nil {
		t.Fatalf("IsCommitMergedToMain: %v", err)
	}
	if merged {
		t.Error("expected feature commit to not be merged to main")
	}
}

func headHash(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}
